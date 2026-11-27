package wxrimport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/migrate/model"
)

// MaxMediaBytes is the largest file downloaded.
const MaxMediaBytes = 20 << 20

// imageTypes are the accepted content types, by sniffed type.
var imageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}

// mediaRef is a media row that body images can point at.
type mediaRef struct {
	id  int64
	key string
	alt string
}

var (
	uploadsDateRe = regexp.MustCompile(`^(\d{4})/(\d{2})/`)
	safeNameRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	sizeSuffixRe  = regexp.MustCompile(`-\d+x\d+(\.[A-Za-z0-9]+)$`)
)

// storageKey builds wp/<yyyy>/<mm>/<filename> from the attachment URL
// (WordPress keeps files under uploads/<yyyy>/<mm>/). Non-ASCII file names
// are replaced by the attachment ID so keys stay URL- and S3-safe.
func storageKey(a model.Attachment) (string, error) {
	u, err := url.Parse(a.URL)
	if err != nil {
		return "", err
	}
	name := path.Base(u.Path)
	ext := strings.ToLower(path.Ext(name))
	if !imageExts[ext] {
		return "", errNotImage
	}
	if !safeNameRe.MatchString(name) {
		name = fmt.Sprintf("%d%s", a.SourceID, ext)
	}
	ym := ""
	if i := strings.Index(u.Path, "/uploads/"); i >= 0 {
		if m := uploadsDateRe.FindStringSubmatch(u.Path[i+len("/uploads/"):]); m != nil {
			ym = m[1] + "/" + m[2]
		}
	}
	if ym == "" {
		if a.Date.IsZero() {
			ym = "undated"
		} else {
			ym = a.Date.Format("2006/01")
		}
	}
	return "wp/" + ym + "/" + name, nil
}

var errNotImage = errors.New("not an image")

// urlKey identifies an image URL regardless of scheme and of WordPress's
// resized variants (photo-300x200.jpg is photo.jpg).
func urlKey(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	p := u.Path
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	return strings.ToLower(u.Host) + sizeSuffixRe.ReplaceAllString(p, "$1")
}

type download struct {
	data          []byte
	size          int64 // len(data), kept after data is dropped
	contentType   string
	width, height *int64
}

// fetch downloads an image, enforcing the size limit and the image types.
func fetch(ctx context.Context, client *http.Client, rawURL string) (*download, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("URL が http(s) ではありません: %s", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ダウンロードできませんでした: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ダウンロードできませんでした（HTTP %d）", resp.StatusCode)
	}
	if resp.ContentLength > MaxMediaBytes {
		return nil, fmt.Errorf("ファイルが大きすぎます（%d バイト、上限 %d バイト）", resp.ContentLength, MaxMediaBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxMediaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("ダウンロードが途中で失敗しました: %w", err)
	}
	if len(data) > MaxMediaBytes {
		return nil, fmt.Errorf("ファイルが大きすぎます（上限 %d バイト）", MaxMediaBytes)
	}
	ct := http.DetectContentType(data)
	if _, ok := imageTypes[ct]; !ok {
		return nil, fmt.Errorf("画像ではありません（%s）", ct)
	}
	d := &download{data: data, contentType: ct}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		w, h := int64(cfg.Width), int64(cfg.Height)
		d.width, d.height = &w, &h
	}
	return d, nil
}

// writeFile stores data under root atomically.
func writeFile(root, name string, data []byte) error {
	dst := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".import-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	// CreateTemp makes 0600 files; the web server reads media through the
	// group, as with uploads (media.Store).
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// preparedMedia is an attachment resolved and downloaded, before any
// transaction: downloads take seconds, and a transaction that has added
// a row holds the tenant's id counter (tenant_id_counters) until it ends,
// which would make the admin wait.
type preparedMedia struct {
	entry Entry
	key   string
	// done means the entry is decided (skipped or failed) without the DB.
	done bool
	// meta is the downloaded image (its file already written unless dry
	// run), nil when media with the key already existed.
	meta *download
	err  error // download or write error, reported if nothing exists at key
}

// prepareMedia resolves an attachment's storage key and, unless media with
// that key exists, downloads it and (unless dry run) writes the file. It
// runs outside any transaction, checking for existing media in a short one.
func (im *importer) prepareMedia(ctx context.Context, a model.Attachment) *preparedMedia {
	p := &preparedMedia{entry: Entry{Kind: KindMedia, SourceID: a.SourceID, Title: a.URL}}
	key, err := storageKey(a)
	switch {
	case errors.Is(err, errNotImage):
		p.done, p.entry.Outcome, p.entry.Reason = true, Skipped, "画像以外のファイルは取り込みません。本文からのリンクは元のURLのままです"
		return p
	case err != nil:
		p.done, p.entry.Outcome, p.entry.Reason = true, Failed, "URL を読み取れません: "+err.Error()
		return p
	}
	if other, dup := im.keys[key]; dup && other != a.SourceID {
		key = fmt.Sprintf("wp/%s/%d-%s", path.Dir(strings.TrimPrefix(key, "wp/")), a.SourceID, path.Base(key))
	}
	im.keys[key] = a.SourceID
	p.key = key
	p.entry.Path = "/media/" + key

	exists := false
	if err := im.db.InTenant(ctx, func(q *dbgen.Queries) error {
		_, err := q.GetMediaByStorageKey(ctx, key)
		exists = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}); err != nil {
		p.done, p.entry.Outcome, p.entry.Reason = true, Failed, err.Error()
		return p
	}
	if exists {
		return p
	}
	d, err := fetch(ctx, im.client, a.URL)
	if err != nil {
		p.err = err
		return p
	}
	if !im.opts.DryRun {
		if err := writeFile(filepath.Join(im.opts.MediaRoot, im.tenantID.String()), key, d.data); err != nil {
			p.err = fmt.Errorf("保存できませんでした: %w", err)
			return p
		}
	}
	d.size, d.data = int64(len(d.data)), nil
	p.meta = d
	return p
}

// recordMedia adds the media row of a prepared attachment.
func (im *importer) recordMedia(ctx context.Context, a model.Attachment, p *preparedMedia) {
	e := p.entry
	if p.done {
		im.rep.add(e)
		return
	}
	var ref mediaRef
	err := im.unit(func(q *dbgen.Queries) error {
		m, err := q.GetMediaByStorageKey(ctx, p.key)
		if err == nil {
			ref = mediaRef{id: m.ID, key: p.key, alt: m.AltText}
			e.Outcome, e.Reason = Existing, "同じ保存先（"+p.key+"）の画像があります"
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if p.err != nil {
			return p.err
		}
		if p.meta == nil {
			return errors.New("同じ保存先の画像が取り込みの途中で削除されました。もう一度取り込んでください")
		}
		m, err = q.CreateMedia(ctx, dbgen.CreateMediaParams{
			StorageKey:  p.key,
			Filename:    path.Base(p.key),
			ContentType: p.meta.contentType,
			SizeBytes:   p.meta.size,
			Width:       p.meta.width,
			Height:      p.meta.height,
			AltText:     a.Alt,
			UploadedBy:  &im.opts.AuthorID,
		})
		if err != nil {
			return err
		}
		ref = mediaRef{id: m.ID, key: p.key, alt: a.Alt}
		e.Outcome = Created
		return nil
	})
	if err != nil {
		e.Outcome, e.Reason = Failed, err.Error()
		im.rep.add(e)
		return
	}
	im.media[urlKey(a.URL)] = ref
	im.mediaByKey[p.key] = ref
	im.rep.add(e)
}

var absURLRe = regexp.MustCompile(`(?:https?:)?//[^\s"'<>()\[\]]+`)

// rewriteMediaURLs points URLs of downloaded attachments (including
// resized variants) at /media/<key>. Only the copy that is converted is
// rewritten; the source body is stored as it was.
func (im *importer) rewriteMediaURLs(body string) string {
	if len(im.media) == 0 {
		return body
	}
	return absURLRe.ReplaceAllStringFunc(body, func(u string) string {
		if ref, ok := im.media[urlKey(u)]; ok {
			return "/media/" + ref.key
		}
		return u
	})
}

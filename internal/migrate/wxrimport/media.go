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
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// importMedia downloads one attachment and records it.
func (im *importer) importMedia(ctx context.Context, a model.Attachment) {
	e := Entry{Kind: KindMedia, SourceID: a.SourceID, Title: a.URL}
	key, err := storageKey(a)
	switch {
	case errors.Is(err, errNotImage):
		e.Outcome, e.Reason = Skipped, "画像以外のファイルは取り込みません。本文からのリンクは元のURLのままです"
		im.rep.add(e)
		return
	case err != nil:
		e.Outcome, e.Reason = Failed, "URL を読み取れません: "+err.Error()
		im.rep.add(e)
		return
	}
	if other, dup := im.keys[key]; dup && other != a.SourceID {
		key = fmt.Sprintf("wp/%s/%d-%s", path.Dir(strings.TrimPrefix(key, "wp/")), a.SourceID, path.Base(key))
	}
	im.keys[key] = a.SourceID
	e.Path = "/media/" + key

	var ref mediaRef
	err = im.unit(func(q *dbgen.Queries) error {
		m, err := q.GetMediaByStorageKey(ctx, key)
		if err == nil {
			ref = mediaRef{id: m.ID, key: key, alt: m.AltText}
			e.Outcome, e.Reason = Existing, "同じ保存先（"+key+"）の画像があります"
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		d, err := fetch(ctx, im.client, a.URL)
		if err != nil {
			return err
		}
		if !im.opts.DryRun {
			if err := writeFile(filepath.Join(im.opts.MediaRoot, im.tenantID.String()), key, d.data); err != nil {
				return fmt.Errorf("保存できませんでした: %w", err)
			}
		}
		m, err = q.CreateMedia(ctx, dbgen.CreateMediaParams{
			StorageKey:  key,
			Filename:    path.Base(key),
			ContentType: d.contentType,
			SizeBytes:   int64(len(d.data)),
			Width:       d.width,
			Height:      d.height,
			AltText:     a.Alt,
			UploadedBy:  &im.opts.AuthorID,
		})
		if err != nil {
			return err
		}
		ref = mediaRef{id: m.ID, key: key, alt: a.Alt}
		e.Outcome = Created
		return nil
	})
	if err != nil {
		e.Outcome, e.Reason = Failed, err.Error()
		im.rep.add(e)
		return
	}
	im.media[urlKey(a.URL)] = ref
	im.mediaByKey[key] = ref
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

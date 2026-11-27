package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

const maxUploadBytes = 10 << 20

var imageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type mediaJSON struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	PreviewURL  string    `json:"previewUrl"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	Width       *int64    `json:"width"`
	Height      *int64    `json:"height"`
	AltText     string    `json:"altText"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toMedia(m dbgen.Medium) mediaJSON {
	return mediaJSON{
		ID: m.ID, URL: "/media/" + m.StorageKey, PreviewURL: "/api/media/" + strconv.FormatInt(m.ID, 10) + "/file",
		Filename: m.Filename, ContentType: m.ContentType, SizeBytes: m.SizeBytes,
		Width: m.Width, Height: m.Height, AltText: m.AltText, CreatedAt: m.CreatedAt,
	}
}

func checkAlt(alt string) (string, error) {
	alt = strings.TrimSpace(alt)
	if alt == "" || len([]rune(alt)) > 500 {
		return "", invalid("alt_required", "画像の説明（代替テキスト）を500文字以内で入力してください。")
	}
	return alt, nil
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request, sess *session) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return &apiError{http.StatusRequestEntityTooLarge, "too_large", "画像は10MBまでです。"}
	}
	alt, err := checkAlt(r.FormValue("altText"))
	if err != nil {
		return err
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return invalid("file_required", "画像ファイルを選んでください。")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}
	if len(data) > maxUploadBytes {
		return &apiError{http.StatusRequestEntityTooLarge, "too_large", "画像は10MBまでです。"}
	}
	ctype := http.DetectContentType(data)
	ext, ok := imageExt[ctype]
	if !ok {
		return invalid("unsupported_type", "使える画像の形式は JPEG・PNG・GIF・WebP です。")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return invalid("unsupported_type", "画像を読み取れませんでした。別のファイルでお試しください。")
	}

	now := time.Now().UTC()
	key := fmt.Sprintf("%04d/%02d/%s%s", now.Year(), now.Month(), uuid.NewString(), ext)
	if err := s.Media.Save(sess.tenantID, key, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("save media: %w", err)
	}
	width, height := int64(cfg.Width), int64(cfg.Height)
	ctx := r.Context()
	var m dbgen.Medium
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if err := s.checkMediaQuota(ctx, q, int64(len(data))); err != nil {
			return err
		}
		m, err = q.CreateMedia(ctx, dbgen.CreateMediaParams{
			StorageKey: key, Filename: cleanFilename(hdr.Filename, ext), ContentType: ctype,
			SizeBytes: int64(len(data)), Width: &width, Height: &height, AltText: alt, UploadedBy: &sess.userID,
		})
		return err
	})
	if err != nil {
		_ = s.Media.Remove(sess.tenantID, key)
		return err
	}
	writeJSON(w, http.StatusCreated, toMedia(m))
	return nil
}

// checkMediaQuota refuses an upload of size bytes that would take the
// tenant past MediaQuotaBytes. It holds the tenant's upload lock until the
// transaction ends, so call it in the transaction that adds the media.
func (s *Server) checkMediaQuota(ctx context.Context, q *dbgen.Queries, size int64) error {
	if s.MediaQuotaBytes <= 0 {
		return nil
	}
	if err := q.LockMediaQuota(ctx); err != nil {
		return err
	}
	used, err := q.MediaTotalBytes(ctx)
	if err != nil {
		return err
	}
	if used+size <= s.MediaQuotaBytes {
		return nil
	}
	return &apiError{http.StatusUnprocessableEntity, "quota_exceeded", fmt.Sprintf(
		"画像を保存できる容量（%s）がいっぱいのため、この画像を追加できません（今の使用量：%s）。"+
			"使っていない古い画像を削除してから、もう一度お試しください。容量を増やしたいときは、サイトの管理者に相談してください。",
		formatMB(s.MediaQuotaBytes), formatMB(used))}
}

// formatMB shows a byte count in MB, rounded up to one decimal.
func formatMB(n int64) string {
	tenths := (n*10 + 1<<20 - 1) >> 20
	if tenths%10 == 0 {
		return fmt.Sprintf("%dMB", tenths/10)
	}
	return fmt.Sprintf("%d.%dMB", tenths/10, tenths%10)
}

func cleanFilename(name, ext string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return "image" + ext
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request, _ *session) error {
	out := []mediaJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListMedia(r.Context())
		for _, m := range rows {
			out = append(out, toMedia(m))
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) mediaFile(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var m dbgen.Medium
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		m, err = q.GetMedia(r.Context(), id)
		return err
	})
	if err != nil {
		return err
	}
	f, err := s.Media.Open(sess.tenantID, m.StorageKey)
	if errors.Is(err, errNotExist) {
		return errNotFound
	}
	if err != nil {
		return fmt.Errorf("open media: %w", err)
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", m.CreatedAt, f)
	return nil
}

// deleteMedia deletes an image that no current, published or scheduled
// revision shows. Older revisions may keep pointing at it.
func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var key string
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.GetMedia(ctx, id); err != nil {
			return err
		}
		inUse, err := q.MediaInUse(ctx, id)
		if err != nil {
			return err
		}
		if inUse {
			return &apiError{Status: http.StatusConflict, Code: "media_in_use", Message: "この画像は記事やページで使われているため削除できません。先に本文から外してください。"}
		}
		key, err = q.DeleteMedia(ctx, id)
		return err
	})
	if err != nil {
		return err
	}
	if err := s.Media.Remove(sess.tenantID, key); err != nil {
		s.Logger.Error("remove media file", "key", key, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) patchMedia(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		AltText string `json:"altText"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	alt, err := checkAlt(in.AltText)
	if err != nil {
		return err
	}
	var m dbgen.Medium
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		m, err = q.UpdateMediaAlt(r.Context(), dbgen.UpdateMediaAltParams{ID: id, AltText: alt})
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toMedia(m))
	return nil
}

package api

import (
	"bytes"
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
		return "", invalid("alt_required", "画像の説明（代替テキスト）を500文字以内で入力してください")
	}
	return alt, nil
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request, sess *session) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return &apiError{http.StatusRequestEntityTooLarge, "too_large", "画像は10MBまでです"}
	}
	alt, err := checkAlt(r.FormValue("altText"))
	if err != nil {
		return err
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return invalid("file_required", "画像ファイルを選んでください")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}
	if len(data) > maxUploadBytes {
		return &apiError{http.StatusRequestEntityTooLarge, "too_large", "画像は10MBまでです"}
	}
	ctype := http.DetectContentType(data)
	ext, ok := imageExt[ctype]
	if !ok {
		return invalid("unsupported_type", "使える画像の形式は JPEG・PNG・GIF・WebP です")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return invalid("unsupported_type", "画像を読み取れませんでした。別のファイルでお試しください")
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

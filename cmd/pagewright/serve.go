package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kom3da/pagewright/internal/adminui"
	"github.com/kom3da/pagewright/internal/api"
	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/mail"
	"github.com/kom3da/pagewright/internal/media"
)

func runServe(ctx context.Context, cfg config.Config) error {
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Pool().Ping(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	sender := mail.SMTP{
		Addr: cfg.SMTPAddr, From: cfg.MailFrom,
		Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, ImplicitTLS: cfg.SMTPImplicitTLS,
		RequireStartTLS: cfg.SMTPRequireTLS,
	}
	riverClient, err := newRiver(d, cfg, sender)
	if err != nil {
		return err
	}

	apiServer := api.New(api.Options{
		DB:              d,
		Jobs:            riverJobs{client: riverClient},
		Mail:            sender,
		PublicBaseURL:   cfg.PublicBaseURL,
		TrustedProxies:  cfg.TrustedProxies,
		Media:           media.Store{Root: cfg.MediaDir()},
		MediaQuotaBytes: cfg.MediaQuotaMB << 20,
		BaseURL:         cfg.BaseURL,
		SecureCookies:   cfg.SecureCookies(),
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/f/", apiServer.FormHandler())
	mux.Handle("/p/", apiServer.PreviewHandler())
	mux.Handle("/admin/", adminui.Handler(adminFS(cfg)))
	mux.Handle("GET /{$}", http.RedirectHandler("/admin/", http.StatusFound))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.Pool().Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := riverClient.Start(ctx); err != nil {
			return fmt.Errorf("start jobs: %w", err)
		}
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return riverClient.Stop(stop)
	})
	g.Go(func() error { return listen(ctx, cfg.Addr, logRequests(secureHeaders(mux))) })
	if cfg.SiteAddr != "" {
		g.Go(func() error { return listen(ctx, cfg.SiteAddr, previewHandler(cfg.SiteDir())) })
		slog.Info("公開サイトのプレビューを起動しました", "url", listenURL(cfg.SiteAddr))
	}
	slog.Info("管理画面を起動しました", "url", cfg.BaseURL+"/admin/", "version", version)
	return g.Wait()
}

// listenURL is the URL a listen address answers at (":8081" is
// http://localhost:8081/).
func listenURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// listen serves h on addr until ctx is done.
func listen(ctx context.Context, addr string, h http.Handler) error {
	// ReadTimeout leaves room for a 10 MB upload on a slow line; idle
	// keep-alive connections are not held forever.
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return err
		}
		if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", logPath(r.URL.Path), "status", rec.status, "duration", time.Since(start))
	})
}

// logPath hides the token of preview links (/p/{token}/...), which opens
// unpublished contents for up to 30 days to whoever reads the logs.
func logPath(p string) string {
	rest, ok := strings.CutPrefix(p, "/p/")
	if !ok {
		return p
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return "/p/[redacted]" + rest[i:]
	}
	return "/p/[redacted]"
}

// secureHeaders keeps the admin out of frames on other sites and stops
// browsers from guessing content types. Handlers may set a stricter
// policy (previews do).
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

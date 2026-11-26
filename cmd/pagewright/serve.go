package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

	riverClient, err := newRiver(d, cfg)
	if err != nil {
		return err
	}

	apiServer := api.New(api.Options{
		DB:            d,
		Builds:        riverBuilds{client: riverClient},
		Mail:          mail.SMTP{Addr: cfg.SMTPAddr, From: cfg.MailFrom},
		Media:         media.Store{Root: cfg.MediaDir()},
		BaseURL:       cfg.BaseURL,
		SecureCookies: cfg.SecureCookies(),
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/admin/", adminui.Handler(adminui.Dir(cfg.AdminDir)))
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
	g.Go(func() error { return listen(ctx, cfg.Addr, logRequests(mux)) })
	if cfg.SiteAddr != "" {
		g.Go(func() error { return listen(ctx, cfg.SiteAddr, previewHandler(cfg.SiteDir())) })
		slog.Info("公開サイトのプレビューを起動しました", "url", cfg.PublicBaseURL+"/")
	}
	slog.Info("管理画面を起動しました", "url", cfg.BaseURL+"/admin/")
	return g.Wait()
}

// listen serves h on addr until ctx is done.
func listen(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
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
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}

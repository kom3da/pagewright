// Package config reads settings from environment variables.
package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DevDatabaseURL is the database of deploy/compose.dev.yaml.
const DevDatabaseURL = "postgres://pagewright:pagewright@localhost:5432/pagewright?sslmode=disable"

// Config holds the settings of the server and the CLI.
type Config struct {
	DatabaseURL        string // PAGEWRIGHT_DATABASE_URL: the app's connection
	MigrateDatabaseURL string // PAGEWRIGHT_MIGRATE_DATABASE_URL: table owner, for migrate and init
	Addr               string // PAGEWRIGHT_ADDR: admin and API
	SiteAddr           string // PAGEWRIGHT_SITE_ADDR: public site preview; empty disables it
	BaseURL            string // PAGEWRIGHT_BASE_URL: where the admin is reachable, for mail links
	PublicBaseURL      string // PAGEWRIGHT_PUBLIC_BASE_URL: the public site, for sitemap and feed
	DataDir            string // PAGEWRIGHT_DATA_DIR: media and built sites
	SMTPAddr           string // PAGEWRIGHT_SMTP_ADDR
	MailFrom           string // PAGEWRIGHT_MAIL_FROM
	AdminDir           string // PAGEWRIGHT_ADMIN_DIR: built admin UI, when not embedded
	Pagefind           string // PAGEWRIGHT_PAGEFIND: Pagefind binary for site search; "off" disables
}

// FromEnv reads Config, using development defaults for unset values.
func FromEnv() Config {
	c := Config{
		DatabaseURL:   env("PAGEWRIGHT_DATABASE_URL", DevDatabaseURL),
		Addr:          env("PAGEWRIGHT_ADDR", ":8080"),
		SiteAddr:      env("PAGEWRIGHT_SITE_ADDR", ":8081"),
		BaseURL:       strings.TrimRight(env("PAGEWRIGHT_BASE_URL", "http://localhost:8080"), "/"),
		PublicBaseURL: strings.TrimRight(env("PAGEWRIGHT_PUBLIC_BASE_URL", "http://localhost:8081"), "/"),
		DataDir:       env("PAGEWRIGHT_DATA_DIR", "data"),
		SMTPAddr:      env("PAGEWRIGHT_SMTP_ADDR", "localhost:1025"),
		MailFrom:      env("PAGEWRIGHT_MAIL_FROM", "Pagewright <no-reply@localhost>"),
		AdminDir:      env("PAGEWRIGHT_ADMIN_DIR", filepath.Join("web", "admin", "dist")),
		Pagefind:      os.Getenv("PAGEWRIGHT_PAGEFIND"),
	}
	switch c.Pagefind {
	case "off":
		c.Pagefind = ""
	case "":
		// The extended build segments Japanese; fall back to the plain one.
		for _, name := range []string{"pagefind_extended", "pagefind"} {
			if p, err := exec.LookPath(name); err == nil {
				c.Pagefind = p
				break
			}
		}
	}
	c.MigrateDatabaseURL = env("PAGEWRIGHT_MIGRATE_DATABASE_URL", c.DatabaseURL)
	return c
}

// MediaDir is where uploaded media are stored.
func (c Config) MediaDir() string { return filepath.Join(c.DataDir, "media") }

// SiteDir is where built sites are written, one directory per tenant.
func (c Config) SiteDir() string { return filepath.Join(c.DataDir, "site") }

// SecureCookies reports whether cookies need the Secure attribute.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

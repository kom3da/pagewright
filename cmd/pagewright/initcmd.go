package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/config"
)

// cliLoginTTL is how long a login URL printed by the CLI stays valid.
const cliLoginTTL = "1 hour"

// runInit creates a site and its first admin, then prints a login URL. It
// runs as the table owner, since the app role cannot create sites or users.
func runInit(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	site := fs.String("site", "", "サイトの名前")
	email := fs.String("email", "", "管理者のメールアドレス")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := strings.TrimSpace(*site)
	addr, err := checkEmail(*email)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("--site にサイトの名前を指定してください")
	}

	conn, err := pgx.Connect(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var token string
	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var tenant, user uuid.UUID
		if err := tx.QueryRow(ctx, "INSERT INTO tenants (name) VALUES ($1) RETURNING id", name).Scan(&tenant); err != nil {
			return fmt.Errorf("create site: %w", err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email RETURNING id`, addr).Scan(&user); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')", tenant, user); err != nil {
			return fmt.Errorf("add member: %w", err)
		}
		t, err := issueLoginToken(ctx, tx, user)
		if err != nil {
			return err
		}
		token = t
		fmt.Printf("サイト「%s」を作りました（ID: %s）\n", name, tenant)
		return nil
	})
	if err != nil {
		return err
	}
	printLoginURL(cfg, addr, token)
	return nil
}

// runLoginURL prints a login URL for an existing user, for when mail does
// not arrive.
func runLoginURL(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("login-url", flag.ContinueOnError)
	email := fs.String("email", "", "メールアドレス")
	if err := fs.Parse(args); err != nil {
		return err
	}
	addr, err := checkEmail(*email)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var token string
	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var user uuid.UUID
		err := tx.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", addr).Scan(&user)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%s のユーザーはいません", addr)
		}
		if err != nil {
			return err
		}
		t, err := issueLoginToken(ctx, tx, user)
		token = t
		return err
	})
	if err != nil {
		return err
	}
	printLoginURL(cfg, addr, token)
	return nil
}

func checkEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if a, err := mail.ParseAddress(s); err != nil || a.Address != s {
		return "", errors.New("--email に正しいメールアドレスを指定してください")
	}
	return s, nil
}

func issueLoginToken(ctx context.Context, tx pgx.Tx, user uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(token))
	_, err := tx.Exec(ctx, "INSERT INTO login_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)",
		h[:], user, cliLoginTTL)
	return token, err
}

func printLoginURL(cfg config.Config, email, token string) {
	fmt.Printf("%s のログイン用URL（1時間有効、1回限り）:\n%s/admin/login/verify?token=%s\n",
		email, cfg.BaseURL, url.QueryEscape(token))
}

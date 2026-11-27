package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/db/dbtest"
)

func TestFinishUniqueViolation(t *testing.T) {
	s := New(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	type body struct{ Code, Message, Field string }
	tests := []struct {
		constraint string
		want       body
	}{
		{"contents_slug_key", body{"conflict", errURLTaken.Message, "slug"}},
		{"routes_tenant_id_path_key", body{"conflict", errURLTaken.Message, "slug"}},
		{"categories_tenant_id_slug_key", body{"conflict", uniqueErrors["categories_tenant_id_slug_key"].err.Message, "slug"}},
		{"tags_tenant_id_slug_key", body{"conflict", uniqueErrors["tags_tenant_id_slug_key"].err.Message, "slug"}},
		{"media_tenant_id_storage_key_key", body{"conflict", errConflict.Message, ""}},
		{"", body{"conflict", errConflict.Message, ""}},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/x", nil)
			s.finish(w, r, fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "23505", ConstraintName: tt.constraint}))
			if w.Code != http.StatusConflict {
				t.Errorf("status = %d, want 409", w.Code)
			}
			var got struct{ Error body }
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got.Error); diff != "" {
				t.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}

// TestUniqueErrorsNameRealConstraints keeps uniqueErrors in step with the
// migrations: a renamed constraint would silently fall back to errConflict.
func TestUniqueErrorsNameRealConstraints(t *testing.T) {
	d := dbtest.New(t)
	for name := range uniqueErrors {
		var n int
		err := d.Admin.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			 WHERE i.indisunique AND c.relname = $1 AND c.relnamespace = 'public'::regnamespace`, name).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("no unique index named %s", name)
		}
	}
}

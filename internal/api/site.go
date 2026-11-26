package api

import (
	"io/fs"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

var errNotExist = fs.ErrNotExist

func (s *Server) buildSite(w http.ResponseWriter, r *http.Request, sess *session) error {
	ctx := r.Context()
	err := s.DB.InTenantTx(ctx, func(tx pgx.Tx, _ *dbgen.Queries) error {
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

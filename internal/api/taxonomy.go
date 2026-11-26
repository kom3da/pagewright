package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/routing"
)

const slugMessage = "ページのURLは英小文字・数字・ハイフンで入力してください（数字だけにはできません）"

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9-]{1,100}$`)
	allDigits   = regexp.MustCompile(`^[0-9]+$`)
)

// checkSlug validates a new slug typed in the admin. Slugs migrated from
// WordPress (e.g. Japanese) are only kept, never typed.
func checkSlug(s string) error {
	if !slugPattern.MatchString(s) || allDigits.MatchString(s) {
		return invalid("invalid_slug", slugMessage)
	}
	return nil
}

type categoryJSON struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	ParentID *int64 `json:"parentId"`
	Position int64  `json:"position"`
}

type tagJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func toCategory(c dbgen.Category) categoryJSON {
	return categoryJSON{ID: c.ID, Name: c.Name, Slug: c.Slug, ParentID: c.ParentID, Position: c.Position}
}

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return "", invalid("invalid_name", "名前を100文字以内で入力してください")
	}
	return name, nil
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request, _ *session) error {
	out := []categoryJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListCategories(r.Context())
		for _, c := range rows {
			out = append(out, toCategory(c))
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request, sess *session) error {
	var in struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		ParentID *int64 `json:"parentId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	name, err := checkName(in.Name)
	if err != nil {
		return err
	}
	if err := checkSlug(in.Slug); err != nil {
		return err
	}
	ctx := r.Context()
	var out categoryJSON
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		c, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{ParentID: in.ParentID, Name: name, Slug: in.Slug})
		if err != nil {
			return err
		}
		if err := routing.MoveCanonical(ctx, q, dbgen.Route{}, false, routing.CategoryPath(c.Slug), dbgen.CreateRouteParams{CategoryID: &c.ID}); err != nil {
			return err
		}
		out = toCategory(c)
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) patchCategory(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     *string         `json:"name"`
		Slug     *string         `json:"slug"`
		ParentID json.RawMessage `json:"parentId"`
		Position *int64          `json:"position"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	p := dbgen.UpdateCategoryParams{ID: id, Position: in.Position}
	if in.Name != nil {
		name, err := checkName(*in.Name)
		if err != nil {
			return err
		}
		p.Name = &name
	}
	if len(in.ParentID) > 0 {
		if err := json.Unmarshal(in.ParentID, &p.ParentID); err != nil {
			return invalid("invalid_parent", "親カテゴリの指定が正しくありません")
		}
		p.SetParent = true
	}

	ctx := r.Context()
	var out categoryJSON
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		cur, err := q.GetCategory(ctx, id)
		if err != nil {
			return err
		}
		if in.Slug != nil && *in.Slug != cur.Slug {
			if err := checkSlug(*in.Slug); err != nil {
				return err
			}
			p.Slug = in.Slug
		}
		if p.SetParent && p.ParentID != nil {
			chain, err := q.CategoryAncestorIDs(ctx, *p.ParentID)
			if err != nil {
				return err
			}
			if len(chain) == 0 {
				return invalid("invalid_parent", "親カテゴリが見つかりません")
			}
			for _, a := range chain {
				if a == id {
					return invalid("invalid_parent", "親カテゴリに、このカテゴリ自身やその下のカテゴリは選べません")
				}
			}
		}
		c, err := q.UpdateCategory(ctx, p)
		if err != nil {
			return err
		}
		if c.Slug != cur.Slug {
			route, err := q.GetCanonicalRouteForCategory(ctx, &id)
			hasRoute := err == nil
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err := routing.MoveCanonical(ctx, q, route, hasRoute, routing.CategoryPath(c.Slug), dbgen.CreateRouteParams{CategoryID: &id}); err != nil {
				return err
			}
		}
		out = toCategory(c)
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// deleteCategory deletes a category; its children move up to its parent.
func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request, sess *session) error {
	return s.deleteTaxonomy(w, r, sess, func(q *dbgen.Queries, ctx context.Context, id int64) (int64, error) {
		if err := q.ReparentCategoryChildren(ctx, id); err != nil {
			return 0, err
		}
		return q.DeleteCategory(ctx, id)
	})
}

func (s *Server) patchTag(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Name *string `json:"name"`
		Slug *string `json:"slug"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	p := dbgen.UpdateTagParams{ID: id}
	if in.Name != nil {
		name, err := checkName(*in.Name)
		if err != nil {
			return err
		}
		p.Name = &name
	}
	ctx := r.Context()
	var out tagJSON
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		cur, err := q.GetTag(ctx, id)
		if err != nil {
			return err
		}
		if in.Slug != nil && *in.Slug != cur.Slug {
			if err := checkSlug(*in.Slug); err != nil {
				return err
			}
			p.Slug = in.Slug
		}
		t, err := q.UpdateTag(ctx, p)
		if err != nil {
			return err
		}
		if t.Slug != cur.Slug {
			route, err := q.GetCanonicalRouteForTag(ctx, &id)
			hasRoute := err == nil
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err := routing.MoveCanonical(ctx, q, route, hasRoute, routing.TagPath(t.Slug), dbgen.CreateRouteParams{TagID: &id}); err != nil {
				return err
			}
		}
		out = tagJSON{ID: t.ID, Name: t.Name, Slug: t.Slug}
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request, sess *session) error {
	return s.deleteTaxonomy(w, r, sess, (*dbgen.Queries).DeleteTag)
}

func (s *Server) deleteTaxonomy(w http.ResponseWriter, r *http.Request, sess *session, del func(*dbgen.Queries, context.Context, int64) (int64, error)) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		n, err := del(q, ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return errNotFound
		}
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request, _ *session) error {
	out := []tagJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListTags(r.Context())
		for _, t := range rows {
			out = append(out, tagJSON{ID: t.ID, Name: t.Name, Slug: t.Slug})
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request, sess *session) error {
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	name, err := checkName(in.Name)
	if err != nil {
		return err
	}
	if err := checkSlug(in.Slug); err != nil {
		return err
	}
	ctx := r.Context()
	var out tagJSON
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		t, err := q.CreateTag(ctx, dbgen.CreateTagParams{Name: name, Slug: in.Slug})
		if err != nil {
			return err
		}
		if err := routing.MoveCanonical(ctx, q, dbgen.Route{}, false, routing.TagPath(t.Slug), dbgen.CreateRouteParams{TagID: &t.ID}); err != nil {
			return err
		}
		out = tagJSON{ID: t.ID, Name: t.Name, Slug: t.Slug}
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

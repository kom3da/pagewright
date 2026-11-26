// Package media stores uploaded files on local disk.
//
// Files live at <Root>/<tenantID>/<storage key>; the public site serves
// them at /media/<storage key>.
package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Store is a directory of media files per tenant.
type Store struct {
	Root string
}

// Path returns the file path of key for tenant. key must be a clean,
// relative, slash-separated path.
func (s Store) Path(tenant uuid.UUID, key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || filepath.Clean(key) != filepath.FromSlash(key) || strings.HasPrefix(key, "..") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	return filepath.Join(s.Root, tenant.String(), filepath.FromSlash(key)), nil
}

// Save writes r to key, creating directories. It fails if the file exists.
func (s Store) Save(tenant uuid.UUID, key string, r io.Reader) error {
	p, err := s.Path(tenant, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(p)
		return err
	}
	return f.Close()
}

// Open opens the file at key.
func (s Store) Open(tenant uuid.UUID, key string) (*os.File, error) {
	p, err := s.Path(tenant, key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// Remove deletes the file at key; a missing file is not an error.
func (s Store) Remove(tenant uuid.UUID, key string) error {
	p, err := s.Path(tenant, key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

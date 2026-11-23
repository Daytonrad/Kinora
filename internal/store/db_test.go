package store

import (
	"errors"
	"path/filepath"
	"testing"
)

// A database written by a newer Kinora (an update rolled back) must not be used silently.
func TestNewerSchemaRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE schema_version SET version=?`, len(migrations)+3); err != nil {
		t.Fatal(err)
	}
	s.Close()

	if _, err := Open(path); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("open newer schema: %v", err)
	}
	AllowNewerSchema = true
	defer func() { AllowNewerSchema = false }()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	s.Close()
}

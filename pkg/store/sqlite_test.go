package store

import (
	"fmt"
	"testing"
	"time"
)

func TestCreateSchema(t *testing.T) {
	// Without "init" flag we expect to fail when db file does not exist
	nonexistent := fmt.Sprintf("/tmp/nonexistent-%d", time.Now().UnixNano())
	s, err := NewSQLiteStore(nonexistent, false)
	if err == nil {
		t.Fatal("expected error")
	}

	if s != nil {
		t.Fatal("expected s to be nil")
	}

	// With init flag set new db file should be created
	s, err = NewSQLiteStore(nonexistent, true)
	if err != nil {
		t.Fatalf("uexpected error: %v", err)
	}

	v := 0
	if err := s.db.QueryRowContext(t.Context(), "select version from schema_version where is_current=1").Scan(&v); err != nil {
		t.Fatalf("failed to query schema version: %v", err)
	}

	if v < 1 {
		t.Fatalf("expected schema version to be >= 1, got %d", v)
	}
}

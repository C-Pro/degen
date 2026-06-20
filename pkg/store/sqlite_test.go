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

	v, err := s.getSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}

	if v < 1 {
		t.Fatalf("expected schema version to be >= 1, got %d", v)
	}
}

package store

import (
	"bytes"
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"text/template"
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

	// A fresh init must land on exactly the current version.
	if v != version.Version {
		t.Fatalf("expected schema version %d, got %d", version.Version, v)
	}
}

// TestMigrateFromPrevVersion builds a database at the previous schema version
// (the schema.tmpl tagged db-release-<N-1> in git) and opens it through the
// store, which must run migrate.tmpl and end up structurally identical to a
// freshly-initialized current database.
//
// The previous schema is read from the git tag rather than a checked-in copy,
// so CI must fetch tags / full history (e.g. actions/checkout fetch-depth: 0)
// for this test to find it. There is no db-release-1 (v1 used schema.sql before
// the templating rework), so this only runs from version 2 onward.
func TestMigrateFromPrevVersion(t *testing.T) {
	prevTag := fmt.Sprintf("db-release-%d", version.Version-1)

	prevSchema, err := exec.Command("git", "show", prevTag+":pkg/store/schema.tmpl").Output()
	if err != nil {
		t.Fatalf("could not read %s:pkg/store/schema.tmpl - is the tag present? "+
			"CI needs full git history (fetch-depth: 0): %v", prevTag, err)
	}

	// Materialize a database at the previous version.
	prevPath := filepath.Join(t.TempDir(), "prev.db")
	prevDB, err := sql.Open("sqlite", prevPath)
	if err != nil {
		t.Fatalf("open prev db: %v", err)
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("prev").Parse(string(prevSchema)))
	prevVersion := struct {
		Version     int
		Description string
	}{version.Version - 1, "previous schema"}
	if err := tmpl.Execute(&buf, prevVersion); err != nil {
		t.Fatalf("render prev schema: %v", err)
	}
	if _, err := prevDB.Exec(buf.String()); err != nil {
		t.Fatalf("create prev schema: %v", err)
	}
	if err := prevDB.Close(); err != nil {
		t.Fatalf("close prev db: %v", err)
	}

	// Opening without init detects version N-1 and runs the migration.
	migrated, err := NewSQLiteStore(prevPath, false)
	if err != nil {
		t.Fatalf("migrate on open: %v", err)
	}

	v, err := migrated.getSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	// The version bump lives in the same transaction as the DDL, so reaching
	// the current version means migrate.tmpl applied end-to-end.
	if v != version.Version {
		t.Fatalf("expected schema version %d after migration, got %d", version.Version, v)
	}

	// The migrated schema must match a fresh install of the current version.
	// This catches drift between migrate.tmpl and schema.tmpl (e.g. a table or
	// index added to one but not the other).
	fresh, err := NewSQLiteStore(filepath.Join(t.TempDir(), "fresh.db"), true)
	if err != nil {
		t.Fatalf("init fresh db: %v", err)
	}

	got := dumpSchema(t, migrated.db)
	want := dumpSchema(t, fresh.db)
	if len(got) != len(want) {
		t.Fatalf("migrated schema has %d objects, fresh has %d:\nmigrated=%v\nfresh=%v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schema object mismatch:\nmigrated: %s\nfresh:    %s", got[i], want[i])
		}
	}
}

func TestOpenUnsupportedVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ancient.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// A schema_version table at a version we can't migrate from (more than one
	// step behind) must be rejected, not silently used.
	setup := fmt.Sprintf(`
create table schema_version(
  version integer primary key,
  description text not null,
  is_current boolean default 0 check (is_current in (0, 1))
);
create unique index schema_version_uk on schema_version(is_current) where is_current = 1;
insert into schema_version(version, description, is_current) values(%d, 'ancient', 1);`,
		version.Version-5)
	if _, err := db.Exec(setup); err != nil {
		t.Fatalf("seed ancient schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if _, err := NewSQLiteStore(dbPath, false); err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
}

// dumpSchema returns the structural definition of every user object in the
// database, ordered deterministically, so two databases can be compared.
func dumpSchema(t *testing.T, db *sql.DB) []string {
	t.Helper()

	rows, err := db.Query(`select type, name, coalesce(sql, '') from sqlite_master ` +
		`where name not like 'sqlite_%' order by type, name`)
	if err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var objs []string
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("scan sqlite_master: %v", err)
		}
		objs = append(objs, fmt.Sprintf("%s %s: %s", typ, name, ddl))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master: %v", err)
	}

	return objs
}

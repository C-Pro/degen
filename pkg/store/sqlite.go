// Package store contains the storage layer implemented as
// a sqlite database
package store

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"text/template"

	_ "embed"

	_ "modernc.org/sqlite"
)

var (
	//go:embed schema.tmpl
	schemaTmpl string
	//go:embed migrate.tmpl
	migrateTmpl string
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStore(fname string, init bool) (*SQLiteStorage, error) {
	dsn := fname
	if !init {
		// mode=rw will fail if fname does not exist as opposed
		// to default mode=crw
		dsn = fmt.Sprintf("file://%s?mode=rw", fname)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database %q: %w", fname, err)
	}

	// Open doesn't really check that the db file exists. We want NewStorage to fail on non-existing db file
	// when not in init mode, so we'll do Ping to check.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to Ping the database %q: %w", fname, err)
	}

	s := &SQLiteStorage{
		db: db,
	}

	if init {
		if err := s.initSchema(); err != nil {
			return nil, err
		}
	} else {
		v, err := s.getSchemaVersion()
		if err != nil {
			return nil, err
		}

		switch v {
		case version.Version:
			// nothing to do
		case version.Version - 1:
			// run migration
			if err := s.migrate(); err != nil {
				return nil, fmt.Errorf("migration failed: %w", err)
			}
		default:
			return nil, fmt.Errorf(
				"database version mismatch: expected %d or %d, but got %d",
				version.Version-1,
				version.Version,
				v)
		}
	}

	return s, nil
}

func (s *SQLiteStorage) initSchema() error {
	var buf bytes.Buffer
	t := template.Must(template.New("schema").Parse(schemaTmpl))
	if err := t.Execute(&buf, version); err != nil {
		return fmt.Errorf("failed to render schema template: %w", err)
	}
	_, err := s.db.Exec(buf.String())
	if err != nil {
		return fmt.Errorf("schema creation failed: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) migrate() error {
	var buf bytes.Buffer
	t := template.Must(template.New("migrate").Parse(migrateTmpl))
	if err := t.Execute(&buf, version); err != nil {
		return fmt.Errorf("migration template render failed: %w", err)
	}
	_, err := s.db.Exec(buf.String())
	if err != nil {
		return fmt.Errorf("schema migration failed: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) getSchemaVersion() (int, error) {
	var v int
	if err := s.db.QueryRowContext(context.TODO(), "select version from schema_version where is_current=1").Scan(&v); err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}

	return v, nil
}

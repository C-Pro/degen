package store

import (
	"database/sql"
	"fmt"

	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

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
	}

	return s, nil
}

func (s *SQLiteStorage) initSchema() error {
	fmt.Println(schema)
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("schema creation failed: %w", err)
	}

	return nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

type Store struct {
	database *sql.DB
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
	existed, err := inspectDatabaseFile(databasePath)
	if err != nil {
		return nil, err
	}
	database, err := sql.Open(sqliteDriverName, sqliteSource(databasePath))
	if err != nil {
		return nil, fmt.Errorf("configure SQLite driver: %w", err)
	}
	opened := &Store{database: database}
	if err := opened.verify(ctx); err != nil {
		return nil, closeAfterOpenFailure(database, err)
	}
	if !existed {
		if err := os.Chmod(databasePath, 0o600); err != nil {
			return nil, closeAfterOpenFailure(database, fmt.Errorf("secure new SQLite database: %w", err))
		}
	}
	return opened, nil
}

func (s *Store) Ready(ctx context.Context) error {
	if err := s.database.PingContext(ctx); err != nil {
		return fmt.Errorf("check SQLite readiness: %w", err)
	}
	return nil
}

func (s *Store) SQLDatabase() *sql.DB {
	return s.database
}

func (s *Store) Close() error {
	if err := s.database.Close(); err != nil {
		return fmt.Errorf("close SQLite database: %w", err)
	}
	return nil
}

func (s *Store) verify(ctx context.Context) error {
	if err := s.Ready(ctx); err != nil {
		return fmt.Errorf("open SQLite database: %w", err)
	}
	if err := verifyDatabaseIntegrity(ctx, s.database); err != nil {
		return err
	}
	return verifyForeignKeys(ctx, s.database)
}

func inspectDatabaseFile(databasePath string) (bool, error) {
	info, err := os.Stat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect SQLite database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("SQLite database path is not a regular file")
	}
	file, err := os.OpenFile(databasePath, os.O_RDWR, 0)
	if err != nil {
		return false, fmt.Errorf("open existing SQLite database without replacement: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close existing SQLite database inspection: %w", err)
	}
	return true, nil
}

func sqliteSource(databasePath string) string {
	cleanPath := filepath.Clean(databasePath)
	source := &url.URL{Scheme: "file", Path: cleanPath}
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	source.RawQuery = query.Encode()
	return source.String()
}

func verifyDatabaseIntegrity(ctx context.Context, database *sql.DB) error {
	var result string
	if err := database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("check SQLite database integrity: %w", err)
	}
	if result != "ok" {
		return errors.New("SQLite database integrity check failed")
	}
	return nil
}

func verifyForeignKeys(ctx context.Context, database *sql.DB) error {
	var enabled int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return fmt.Errorf("verify SQLite foreign keys: %w", err)
	}
	if enabled != 1 {
		return errors.New("SQLite foreign-key enforcement is disabled")
	}
	return nil
}

func closeAfterOpenFailure(database *sql.DB, openError error) error {
	closeError := database.Close()
	return errors.Join(openError, wrapCloseError(closeError))
}

func wrapCloseError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("close failed SQLite database: %w", err)
}

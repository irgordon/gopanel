package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

const migrationDirectory = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

type migration struct {
	version int
	name    string
	query   string
}

type appliedMigration struct {
	version int
	name    string
}

func (s *Store) Migrate(ctx context.Context) error {
	return s.migrate(ctx, embeddedMigrations, migrationDirectory)
}

func (s *Store) migrate(ctx context.Context, migrationFiles fs.FS, directory string) error {
	migrations, err := loadMigrations(migrationFiles, directory)
	if err != nil {
		return err
	}
	if err := ensureMigrationTable(ctx, s.database); err != nil {
		return err
	}
	applied, err := readAppliedMigrations(ctx, s.database)
	if err != nil {
		return err
	}
	if err := validateAppliedMigrations(migrations, applied); err != nil {
		return err
	}
	return applyPendingMigrations(ctx, s.database, migrations, applied)
}

func loadMigrations(migrationFiles fs.FS, directory string) ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, directory)
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations, err := parseMigrationEntries(migrationFiles, directory, entries)
	if err != nil {
		return nil, err
	}
	sort.Slice(migrations, func(left int, right int) bool {
		return migrations[left].version < migrations[right].version
	})
	if err := validateMigrationSequence(migrations); err != nil {
		return nil, err
	}
	return migrations, nil
}

func parseMigrationEntries(migrationFiles fs.FS, directory string, entries []fs.DirEntry) ([]migration, error) {
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".sql" {
			continue
		}
		parsed, err := parseMigration(migrationFiles, directory, entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, parsed)
	}
	return migrations, nil
}

func parseMigration(migrationFiles fs.FS, directory string, filename string) (migration, error) {
	version, err := parseMigrationVersion(filename)
	if err != nil {
		return migration{}, err
	}
	contents, err := fs.ReadFile(migrationFiles, path.Join(directory, filename))
	if err != nil {
		return migration{}, fmt.Errorf("read migration %q: %w", filename, err)
	}
	query := strings.TrimSpace(string(contents))
	if query == "" {
		return migration{}, fmt.Errorf("migration %q is empty", filename)
	}
	return migration{version: version, name: filename, query: query}, nil
}

func parseMigrationVersion(filename string) (int, error) {
	base := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 || len(parts[0]) != 4 || parts[1] == "" {
		return 0, fmt.Errorf("migration %q must use NNNN_description.sql", filename)
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil || version < 1 {
		return 0, fmt.Errorf("migration %q has an unsupported version", filename)
	}
	return version, nil
}

func validateMigrationSequence(migrations []migration) error {
	for index, candidate := range migrations {
		expectedVersion := index + 1
		if candidate.version != expectedVersion {
			return fmt.Errorf("migration sequence requires version %04d, found %04d", expectedVersion, candidate.version)
		}
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, database *sql.DB) error {
	const query = `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        applied_at TEXT NOT NULL
    )`
	if _, err := database.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create migration metadata: %w", err)
	}
	return nil
}

func readAppliedMigrations(ctx context.Context, database *sql.DB) ([]appliedMigration, error) {
	rows, err := database.QueryContext(ctx, "SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("read migration metadata: %w", err)
	}
	defer rows.Close()
	return scanAppliedMigrations(rows)
}

func scanAppliedMigrations(rows *sql.Rows) ([]appliedMigration, error) {
	applied := []appliedMigration{}
	for rows.Next() {
		var record appliedMigration
		if err := rows.Scan(&record.version, &record.name); err != nil {
			return nil, fmt.Errorf("scan migration metadata: %w", err)
		}
		applied = append(applied, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration metadata: %w", err)
	}
	return applied, nil
}

func validateAppliedMigrations(migrations []migration, applied []appliedMigration) error {
	if len(applied) > len(migrations) {
		return errors.New("SQLite migration version is newer than this GoPanel build")
	}
	for index, record := range applied {
		expected := migrations[index]
		if record.version != expected.version || record.name != expected.name {
			return fmt.Errorf("SQLite migration metadata is incompatible at version %04d", record.version)
		}
	}
	return nil
}

func applyPendingMigrations(ctx context.Context, database *sql.DB, migrations []migration, applied []appliedMigration) error {
	for _, candidate := range migrations[len(applied):] {
		if err := applyMigration(ctx, database, candidate); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, database *sql.DB, candidate migration) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %04d: %w", candidate.version, err)
	}
	if err := executeMigration(ctx, transaction, candidate); err != nil {
		return rollbackMigration(transaction, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %04d: %w", candidate.version, err)
	}
	return nil
}

func executeMigration(ctx context.Context, transaction *sql.Tx, candidate migration) error {
	if _, err := transaction.ExecContext(ctx, candidate.query); err != nil {
		return fmt.Errorf("execute migration %04d: %w", candidate.version, err)
	}
	const recordQuery = `INSERT INTO schema_migrations (version, name, applied_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)`
	if _, err := transaction.ExecContext(ctx, recordQuery, candidate.version, candidate.name); err != nil {
		return fmt.Errorf("record migration %04d: %w", candidate.version, err)
	}
	return nil
}

func rollbackMigration(transaction *sql.Tx, migrationError error) error {
	rollbackError := transaction.Rollback()
	if rollbackError == nil {
		return migrationError
	}
	return errors.Join(migrationError, fmt.Errorf("rollback failed migration: %w", rollbackError))
}

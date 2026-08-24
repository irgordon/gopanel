package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestOpenCreatesDatabaseOnlyAtConfiguredPath(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open first-run SQLite database: %v", err)
	}
	closeStore(t, database)

	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("inspect created database: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %04o", info.Mode().Perm())
	}
}

func TestOpenReusesExistingDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database := openTestStore(t, databasePath)
	if _, err := database.database.Exec("CREATE TABLE preserved (value TEXT NOT NULL)"); err != nil {
		t.Fatalf("create preserved table: %v", err)
	}
	if _, err := database.database.Exec("INSERT INTO preserved (value) VALUES ('original')"); err != nil {
		t.Fatalf("write preserved row: %v", err)
	}
	closeStore(t, database)

	reopened := openTestStore(t, databasePath)
	defer closeStore(t, reopened)

	var value string
	if err := reopened.database.QueryRow("SELECT value FROM preserved").Scan(&value); err != nil {
		t.Fatalf("read preserved row: %v", err)
	}
	if value != "original" {
		t.Fatalf("expected preserved value, got %q", value)
	}
}

func TestOpenRejectsCorruptDatabaseWithoutChangingBytes(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	original := []byte("not a SQLite database\x00with preserved bytes")
	if err := os.WriteFile(databasePath, original, 0o600); err != nil {
		t.Fatalf("write corrupt fixture: %v", err)
	}
	before := sha256.Sum256(original)

	_, err := Open(context.Background(), databasePath)
	if err == nil {
		t.Fatal("expected corrupt database to fail")
	}

	afterBytes, readError := os.ReadFile(databasePath)
	if readError != nil {
		t.Fatalf("read corrupt fixture after failure: %v", readError)
	}
	after := sha256.Sum256(afterBytes)
	t.Logf("corrupt fixture SHA-256 before=%x after=%x", before, after)
	if before != after {
		t.Fatalf("corrupt database changed: before=%x after=%x", before, after)
	}
}

func TestOpenRejectsUnreadableDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	if err := os.WriteFile(databasePath, nil, 0o600); err != nil {
		t.Fatalf("write database fixture: %v", err)
	}
	if err := os.Chmod(databasePath, 0); err != nil {
		t.Fatalf("make database unreadable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(databasePath, 0o600); err != nil {
			t.Errorf("restore database permissions: %v", err)
		}
	})

	_, err := Open(context.Background(), databasePath)
	if err == nil {
		t.Fatal("expected unreadable database to fail")
	}
}

func TestEmbeddedMigrationsCreateNoCapabilityTables(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)

	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("run embedded migrations: %v", err)
	}

	assertMigrationCount(t, database, 3)
	assertExpectedPhase3Tables(t, database)
}

func TestMigrateSupportsZeroApplicationMigrations(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)

	files := fstest.MapFS{"migrations/README.md": {Data: []byte("no application migrations")}}
	if err := database.migrate(context.Background(), files, "migrations"); err != nil {
		t.Fatalf("run zero migrations: %v", err)
	}

	assertMigrationCount(t, database, 0)
	assertOnlyMigrationMetadataTable(t, database)
}

func TestMigrateAppliesDeterministicOrder(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)

	files := migrationFS(
		"0002_second.sql", "INSERT INTO migration_order (version) VALUES (2);",
		"0001_first.sql", "CREATE TABLE migration_order (version INTEGER NOT NULL); INSERT INTO migration_order (version) VALUES (1);",
	)
	if err := database.migrate(context.Background(), files, "migrations"); err != nil {
		t.Fatalf("run ordered migrations: %v", err)
	}

	rows, err := database.database.Query("SELECT version FROM migration_order ORDER BY rowid")
	if err != nil {
		t.Fatalf("read migration order: %v", err)
	}
	defer rows.Close()

	var observed []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scan migration order: %v", err)
		}
		observed = append(observed, version)
	}
	if fmt.Sprint(observed) != "[1 2]" {
		t.Fatalf("expected [1 2], got %v", observed)
	}
}

func TestMigrateDoesNotReapplyRecordedMigration(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)
	files := migrationFS("0001_once.sql", "CREATE TABLE apply_once (id INTEGER PRIMARY KEY);")

	if err := database.migrate(context.Background(), files, "migrations"); err != nil {
		t.Fatalf("run first migration pass: %v", err)
	}
	if err := database.migrate(context.Background(), files, "migrations"); err != nil {
		t.Fatalf("run second migration pass: %v", err)
	}

	assertMigrationCount(t, database, 1)
}

func TestMigrateRejectsBrokenMigrationWithoutRecordingIt(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)
	files := migrationFS("0001_broken.sql", "CREATE TABLE unfinished (id INTEGER); THIS IS INVALID SQL;")

	err := database.migrate(context.Background(), files, "migrations")
	if err == nil {
		t.Fatal("expected broken migration to fail")
	}
	if !strings.Contains(err.Error(), "execute migration 0001") {
		t.Fatalf("expected migration context, got %q", err)
	}

	assertMigrationCount(t, database, 0)
	assertTableMissing(t, database, "unfinished")
}

func TestLoadMigrationsRejectsInvalidVersions(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		want  string
	}{
		{name: "malformed", files: migrationFS("bad.sql", "SELECT 1;"), want: "NNNN_description.sql"},
		{name: "unsupported", files: migrationFS("0000_zero.sql", "SELECT 1;"), want: "unsupported version"},
		{name: "duplicate", files: migrationFS("0001_first.sql", "SELECT 1;", "0001_duplicate.sql", "SELECT 1;"), want: "requires version 0002"},
		{name: "missing", files: migrationFS("0001_first.sql", "SELECT 1;", "0003_third.sql", "SELECT 1;"), want: "requires version 0002"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadMigrations(test.files, "migrations")
			if err == nil {
				t.Fatal("expected invalid migrations to fail")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %q", test.want, err)
			}
		})
	}
}

func TestMigrateRejectsFutureDatabaseVersion(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)
	files := migrationFS("0001_current.sql", "SELECT 1;")

	if err := database.migrate(context.Background(), files, "migrations"); err != nil {
		t.Fatalf("run current migration: %v", err)
	}
	if _, err := database.database.Exec(
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (2, '0002_future.sql', CURRENT_TIMESTAMP)",
	); err != nil {
		t.Fatalf("record future migration: %v", err)
	}

	err := database.migrate(context.Background(), files, "migrations")
	if err == nil || !strings.Contains(err.Error(), "newer than this GoPanel build") {
		t.Fatalf("expected future-version rejection, got %v", err)
	}
}

func TestForeignKeyEnforcementAppliesToDatabaseConnections(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	defer closeStore(t, database)

	if _, err := database.database.Exec("CREATE TABLE parent (id INTEGER PRIMARY KEY); CREATE TABLE child (parent_id INTEGER REFERENCES parent(id));"); err != nil {
		t.Fatalf("create foreign-key fixture: %v", err)
	}
	if _, err := database.database.Exec("INSERT INTO child (parent_id) VALUES (999)"); err == nil {
		t.Fatal("expected foreign-key violating insert to fail")
	}
}

func TestReadyFailsAfterDatabaseClose(t *testing.T) {
	database := openTestStore(t, filepath.Join(t.TempDir(), "gopanel.db"))
	closeStore(t, database)

	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("expected closed database to be not ready")
	}
}

func openTestStore(t *testing.T, databasePath string) *Store {
	t.Helper()
	database, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	return database
}

func closeStore(t *testing.T, database *Store) {
	t.Helper()
	if err := database.Close(); err != nil {
		t.Fatalf("close test database: %v", err)
	}
}

func migrationFS(nameAndSQL ...string) fstest.MapFS {
	files := fstest.MapFS{}
	for index := 0; index < len(nameAndSQL); index += 2 {
		files[filepath.ToSlash(filepath.Join("migrations", nameAndSQL[index]))] = &fstest.MapFile{
			Data: []byte(nameAndSQL[index+1]),
			Mode: fs.FileMode(0o644),
		}
	}
	return files
}

func assertMigrationCount(t *testing.T, database *Store, expected int) {
	t.Helper()
	var count int
	if err := database.database.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d migrations, got %d", expected, count)
	}
}

func assertOnlyMigrationMetadataTable(t *testing.T, database *Store) {
	t.Helper()
	rows, err := database.database.Query("SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	if fmt.Sprint(tables) != "[schema_migrations]" {
		t.Fatalf("expected only migration metadata, got %v", tables)
	}
}

func assertExpectedPhase2Tables(t *testing.T, database *Store) {
	t.Helper()
	rows, err := database.database.Query("SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	expected := "[schema_migrations sessions users]"
	if fmt.Sprint(tables) != expected {
		t.Fatalf("expected %s, got %v", expected, tables)
	}
}

func assertExpectedPhase3Tables(t *testing.T, database *Store) {
	t.Helper()
	rows, err := database.database.Query("SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	expected := "[audit_log schema_migrations servers sessions users]"
	if fmt.Sprint(tables) != expected {
		t.Fatalf("expected %s, got %v", expected, tables)
	}
}

func assertTableMissing(t *testing.T, database *Store, name string) {
	t.Helper()
	var count int
	if err := database.database.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
		t.Fatalf("check table absence: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected table %q to be absent", name)
	}
}

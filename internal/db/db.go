// Package db opens the SQLite database and applies the embedded migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (creating if needed) the database at path and migrates it.
// Before migrating a database that already has data, it writes a backup
// next to the regular ones in backupDir.
func Open(path, backupDir string) (*sql.DB, error) {
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db, backupDir); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB, backupDir string) error {
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(names) {
		return fmt.Errorf("database schema version %d is newer than this binary (%d)", current, len(names))
	}
	if current == len(names) {
		return nil
	}
	if current > 0 && backupDir != "" {
		if _, err := Backup(db, backupDir, "pre-migration"); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
	}
	for i := current; i < len(names); i++ {
		version, err := strconv.Atoi(strings.SplitN(filepath.Base(names[i]), "_", 2)[0])
		if err != nil || version != i+1 {
			return fmt.Errorf("migration %s is out of sequence", names[i])
		}
		body, err := migrations.ReadFile(names[i])
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", names[i], err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup writes a consistent copy of the database to dir and returns its path.
func Backup(db *sql.DB, dir, label string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("strapontin-%s-%s.db", time.Now().UTC().Format("20060102-150405"), label))
	_, err := db.ExecContext(context.Background(), "VACUUM INTO ?", path)
	return path, err
}

// Now returns the current time in the format stored in every timestamp column.
func Now() string { return time.Now().UTC().Format(time.RFC3339) }

package db

import (
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndReopens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := Open(path, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	var version int
	d.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
	var fk int
	d.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys are off")
	}
	d.Close()

	d, err = Open(path, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	d.Close()
}

func TestSearchTriggersFoldAccents(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(`INSERT INTO production (title, created_at) VALUES ('Bérénice', ?)`, Now()); err != nil {
		t.Fatal(err)
	}
	var n int
	d.QueryRow(`SELECT count(*) FROM search WHERE search MATCH 'berenice'`).Scan(&n)
	if n != 1 {
		t.Fatalf("matches = %d, want 1", n)
	}
	d.Exec(`UPDATE production SET title = 'Phèdre'`)
	d.QueryRow(`SELECT count(*) FROM search WHERE search MATCH 'berenice'`).Scan(&n)
	if n != 0 {
		t.Fatal("search not updated")
	}
	d.Exec(`DELETE FROM production`)
	d.QueryRow(`SELECT count(*) FROM search`).Scan(&n)
	if n != 0 {
		t.Fatal("search row not deleted")
	}
}

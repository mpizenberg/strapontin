package db

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// KeepDaily is how many daily backups are kept.
const KeepDaily = 14

// DailyBackups writes a backup when the newest daily one is more than a day
// old, checking every hour until ctx ends, and keeps the last KeepDaily.
func DailyBackups(ctx context.Context, d *sql.DB, dir string) {
	for {
		if err := dailyBackup(d, dir, time.Now()); err != nil {
			log.Printf("daily backup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func dailyBackup(d *sql.DB, dir string, now time.Time) error {
	files, _ := filepath.Glob(filepath.Join(dir, "strapontin-*-daily.db"))
	sort.Strings(files) // names start with a UTC timestamp
	if n := len(files); n > 0 {
		if st, err := os.Stat(files[n-1]); err == nil && now.Sub(st.ModTime()) < 24*time.Hour {
			return nil
		}
	}
	path, err := Backup(d, dir, "daily")
	if err != nil {
		return err
	}
	files = append(files, path)
	for _, old := range files[:max(0, len(files)-KeepDaily)] {
		os.Remove(old)
	}
	return nil
}

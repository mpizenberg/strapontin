package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mpizenberg/strapontin/internal/db"
)

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse")
	if !CheckPassword(h, "correct horse") {
		t.Fatal("right password rejected")
	}
	if CheckPassword(h, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword("garbage", "x") {
		t.Fatal("garbage hash accepted")
	}
}

func TestLimiter(t *testing.T) {
	l := &Limiter{Max: 2, Window: time.Minute, attempts: map[string][]time.Time{}}
	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("first attempts refused")
	}
	if l.Allow("a") {
		t.Fatal("third attempt allowed")
	}
	if !l.Allow("b") {
		t.Fatal("other key refused")
	}
}

func TestSessionLifecycle(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	id, err := CreateMember(d, "Lucie", "lucie", "secret-pass", "member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := CheckLogin(d, "lucie", "nope"); m != nil {
		t.Fatal("bad password accepted")
	}
	m, err := CheckLogin(d, "lucie", "secret-pass")
	if err != nil || m == nil || m.ID != id {
		t.Fatalf("login failed: %v %v", m, err)
	}
	token, err := CreateSession(d, id, "test")
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := LookupSession(d, token); m == nil || m.Name != "Lucie" {
		t.Fatal("session not found")
	}
	d.Exec(`UPDATE member SET disabled_at = ?`, db.Now())
	if m, _ := LookupSession(d, token); m != nil {
		t.Fatal("disabled member still signed in")
	}
	d.Exec(`UPDATE member SET disabled_at = NULL`)
	DeleteSession(d, token)
	if m, _ := LookupSession(d, token); m != nil {
		t.Fatal("deleted session still valid")
	}
}

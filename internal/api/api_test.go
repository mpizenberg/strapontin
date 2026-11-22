package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// testEnv is a server on a fresh database with one member and one admin.
type testEnv struct {
	t      *testing.T
	db     *sql.DB
	srv    *httptest.Server
	member int64
	admin  int64
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	admin, _ := auth.CreateMember(d, "Admin", "admin", "admin-pass", "admin", nil)
	member, _ := auth.CreateMember(d, "Lucie Martin", "lucie", "lucie-pass", "member", &admin)
	srv := httptest.NewServer(New(d, Config{BaseURL: "http://example.test", AllowPrivateFetch: true}, nil))
	t.Cleanup(srv.Close)
	return &testEnv{t: t, db: d, srv: srv, member: member, admin: admin}
}

// client returns an HTTP client signed in as login, or anonymous when login is empty.
func (e *testEnv) client(login string) *client {
	jar, _ := cookiejar.New(nil)
	c := &client{env: e, http: &http.Client{Jar: jar}}
	if login != "" {
		c.do("POST", "/api/login", map[string]string{"login": login, "password": login + "-pass"}, http.StatusOK, nil)
	}
	return c
}

type client struct {
	env  *testEnv
	http *http.Client
}

// do sends a JSON request, checks the status and decodes the response into out.
func (c *client) do(method, path string, body any, want int, out any) {
	c.env.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.env.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.env.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		var e map[string]any
		json.NewDecoder(resp.Body).Decode(&e)
		c.env.t.Fatalf("%s %s: status %d, want %d (%v)", method, path, resp.StatusCode, want, e)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.env.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
}

func TestLoginFlow(t *testing.T) {
	e := newTestEnv(t)
	anon := e.client("")
	anon.do("GET", "/api/me", nil, http.StatusUnauthorized, nil)
	anon.do("POST", "/api/login", map[string]string{"login": "lucie", "password": "bad"}, http.StatusUnauthorized, nil)

	c := e.client("lucie")
	var me auth.Member
	c.do("GET", "/api/me", nil, http.StatusOK, &me)
	if me.Name != "Lucie Martin" || me.Role != "member" {
		t.Fatalf("me = %+v", me)
	}
	c.do("POST", "/api/logout", nil, http.StatusNoContent, nil)
	c.do("GET", "/api/me", nil, http.StatusUnauthorized, nil)
}

func TestLoginRateLimit(t *testing.T) {
	e := newTestEnv(t)
	c := e.client("")
	for range 10 {
		c.do("POST", "/api/login", map[string]string{"login": "lucie", "password": "bad"}, http.StatusUnauthorized, nil)
	}
	c.do("POST", "/api/login", map[string]string{"login": "lucie", "password": "lucie-pass"}, http.StatusTooManyRequests, nil)
}

func TestCrossOriginPostRefused(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/login", bytes.NewBufferString(`{}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	e := newTestEnv(t)
	resp, err := http.Get(e.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("status %d, CSP %q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}
}

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mpizenberg/strapontin/internal/db"
)

func token(link inviteLink) string { return link.URL[strings.LastIndex(link.URL, "/")+1:] }

func TestInvitesAndAdmin(t *testing.T) {
	e := newTestEnv(t)
	var mu sync.Mutex
	var pushes []string
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		pushes = append(pushes, r.URL.Path+" "+string(b))
		mu.Unlock()
	}))
	defer ntfy.Close()
	e.api.ntfyURL = ntfy.URL
	admin := e.client("admin")
	admin.do("PUT", "/api/admin/settings", map[string]any{"max_members": 3, "ntfy_topic": "strapontin-test"}, http.StatusNoContent, nil)

	// A member has one invite.
	lucie := e.client("lucie")
	var me struct{ InvitesLeft int `json:"invites_left"` }
	lucie.do("GET", "/api/me", nil, http.StatusOK, &me)
	if me.InvitesLeft != 1 {
		t.Fatalf("invites left = %d", me.InvitesLeft)
	}
	var link inviteLink
	lucie.do("POST", "/api/invites", nil, http.StatusCreated, &link)
	if !strings.HasPrefix(link.URL, "http://example.test/invitation/") {
		t.Fatalf("link = %+v", link)
	}
	lucie.do("POST", "/api/invites", nil, http.StatusForbidden, nil)

	// Redeeming creates the account and signs in.
	guest := e.client("")
	var info map[string]string
	guest.do("GET", "/api/invites/"+token(link), nil, http.StatusOK, &info)
	if info["kind"] != "signup" || info["invited_by"] != "Lucie Martin" {
		t.Fatalf("invite info = %v", info)
	}
	guest.do("POST", "/api/invites/"+token(link)+"/redeem", map[string]string{"name": "Paul", "login": "paul", "password": "short"}, http.StatusBadRequest, nil)
	guest.do("POST", "/api/invites/"+token(link)+"/redeem", map[string]string{"name": "Paul", "login": "lucie", "password": "paul-pass"}, http.StatusBadRequest, nil)
	guest.do("POST", "/api/invites/"+token(link)+"/redeem", map[string]string{"name": "Paul  Roux", "login": "Paul", "password": "paul-pass"}, http.StatusOK, nil)
	var paul struct{ Name, Login string }
	guest.do("GET", "/api/me", nil, http.StatusOK, &paul)
	if paul.Name != "Paul Roux" || paul.Login != "paul" {
		t.Fatalf("new member = %+v", paul)
	}
	guest.do("POST", "/api/invites/"+token(link)+"/redeem", map[string]string{"name": "X", "login": "x", "password": "x-pass-long"}, http.StatusNotFound, nil)
	e.client("").do("GET", "/api/invites/"+token(link), nil, http.StatusNotFound, nil)

	// The cap (3 members) refuses the next signup and tells the admin.
	var adminLink inviteLink
	admin.do("POST", "/api/invites", nil, http.StatusCreated, &adminLink)
	e.client("").do("POST", "/api/invites/"+token(adminLink)+"/redeem", map[string]string{"name": "Zoé", "login": "zoe", "password": "zoe-pass-long"}, http.StatusBadRequest, nil)

	// Admin pages: members with invite depth, overview, members can't see them.
	var members []adminMember
	admin.do("GET", "/api/admin/members", nil, http.StatusOK, &members)
	if len(members) != 3 || members[2].Login != "paul" || members[2].Depth != 2 || members[2].InvitedBy != "Lucie Martin" {
		t.Fatalf("members = %+v", members)
	}
	var o overview
	admin.do("GET", "/api/admin/overview", nil, http.StatusOK, &o)
	if o.Members != 3 || o.MaxDepth != 2 || o.Settings.MaxMembers != 3 || o.DBBytes == 0 {
		t.Fatalf("overview = %+v", o)
	}
	lucie.do("GET", "/api/admin/overview", nil, http.StatusForbidden, nil)

	// Suspending signs the member out; a reset link sets a new password.
	paulID := members[2].ID
	admin.do("POST", "/api/admin/members/"+itoa(paulID)+"/disable", nil, http.StatusNoContent, nil)
	guest.do("GET", "/api/me", nil, http.StatusUnauthorized, nil)
	guest.do("POST", "/api/login", map[string]string{"login": "paul", "password": "paul-pass"}, http.StatusUnauthorized, nil)
	admin.do("POST", "/api/admin/members/"+itoa(paulID)+"/enable", nil, http.StatusNoContent, nil)
	var reset inviteLink
	admin.do("POST", "/api/admin/members/"+itoa(paulID)+"/reset", nil, http.StatusCreated, &reset)
	e.client("").do("GET", "/api/invites/"+token(reset), nil, http.StatusOK, &info)
	if info["kind"] != "reset" || info["login"] != "paul" {
		t.Fatalf("reset info = %v", info)
	}
	again := e.client("")
	again.do("POST", "/api/invites/"+token(reset)+"/redeem", map[string]string{"password": "new-paul-pass"}, http.StatusOK, nil)
	again.do("POST", "/api/login", map[string]string{"login": "paul", "password": "new-paul-pass"}, http.StatusOK, nil)
	admin.do("POST", "/api/admin/members/"+itoa(e.admin)+"/disable", nil, http.StatusBadRequest, nil)

	// Pushes are sent in the background.
	for range 50 {
		mu.Lock()
		n := len(pushes)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(pushes) != 2 || !strings.HasPrefix(pushes[0], "/strapontin-test ") || strings.Contains(strings.Join(pushes, " "), "Paul") {
		t.Fatalf("ntfy pushes = %q", pushes)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestSetupLink(t *testing.T) {
	if link := newTestEnv(t).api.SetupLink(); link != "" {
		t.Fatalf("setup link with existing accounts: %q", link)
	}

	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d, Config{BaseURL: "http://example.test"}, nil)
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	e := &testEnv{t: t, db: d, api: s, srv: srv}
	link := s.SetupLink()
	if !strings.HasPrefix(link, "http://example.test/invitation/") {
		t.Fatalf("setup link = %q", link)
	}
	tok := link[strings.LastIndex(link, "/")+1:]

	guest := e.client("")
	var info map[string]string
	guest.do("GET", "/api/invites/"+tok, nil, http.StatusOK, &info)
	if info["kind"] != "setup" {
		t.Fatalf("setup info = %v", info)
	}
	var me struct{ Login, Role string }
	guest.do("POST", "/api/invites/"+tok+"/redeem", map[string]string{"name": "Ada", "login": "ada", "password": "ada-pass-long"}, http.StatusOK, &me)
	if me.Login != "ada" || me.Role != "admin" {
		t.Fatalf("first account = %+v", me)
	}
	guest.do("GET", "/api/admin/overview", nil, http.StatusOK, nil)

	// Once an account exists, the link is spent.
	e.client("").do("GET", "/api/invites/"+tok, nil, http.StatusNotFound, nil)
	e.client("").do("POST", "/api/invites/"+tok+"/redeem", map[string]string{"name": "Eve", "login": "eve", "password": "eve-pass-long"}, http.StatusNotFound, nil)
}

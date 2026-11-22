// Package api serves the JSON API and the embedded frontend.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/extract"
	"github.com/mpizenberg/strapontin/internal/sources"
)

// Config holds what the handlers need from the command line.
type Config struct {
	BaseURL    string
	TrustProxy bool
	// AllowPrivateFetch lets imports reach local addresses. Tests only.
	AllowPrivateFetch bool
	// BackupDir is where daily backups go; the admin page shows the latest.
	BackupDir string
}

// Server owns the dependencies shared by all handlers.
type Server struct {
	db      *sql.DB
	cfg     Config
	static  fs.FS
	logins  *auth.Limiter
	fetcher *extract.Fetcher
	sources *sources.Client
	ntfyURL string
	handler http.Handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// New returns the root handler. static is the frontend build; it may be nil in tests.
func New(db *sql.DB, cfg Config, static fs.FS) *Server {
	fetcher := extract.NewFetcher(cfg.AllowPrivateFetch)
	s := &Server{db: db, cfg: cfg, static: static, logins: auth.NewLoginLimiter(), fetcher: fetcher, sources: sources.New(fetcher), ntfyURL: "https://ntfy.sh"}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.member(s.logout))
	mux.HandleFunc("GET /api/me", s.member(s.me))
	mux.HandleFunc("DELETE /api/me", s.member(s.deleteAccount))
	mux.HandleFunc("GET /api/export.json", s.member(s.exportJSON))
	mux.HandleFunc("GET /api/export.csv", s.member(s.exportCSV))
	mux.HandleFunc("POST /api/invites", s.member(s.createInvite))
	mux.HandleFunc("GET /api/invites/{token}", s.checkInvite)
	mux.HandleFunc("POST /api/invites/{token}/redeem", s.redeemInvite)

	mux.HandleFunc("GET /api/admin/overview", s.admin(s.adminOverview))
	mux.HandleFunc("GET /api/admin/members", s.admin(s.adminMembers))
	mux.HandleFunc("POST /api/admin/members/{id}/disable", s.admin(s.adminSetDisabled(true)))
	mux.HandleFunc("POST /api/admin/members/{id}/enable", s.admin(s.adminSetDisabled(false)))
	mux.HandleFunc("POST /api/admin/members/{id}/reset", s.admin(s.adminReset))
	mux.HandleFunc("PUT /api/admin/settings", s.admin(s.adminSettings))
	mux.HandleFunc("GET /api/roles", s.roles)
	mux.HandleFunc("GET /api/search", s.member(s.search))

	mux.HandleFunc("POST /api/import", s.member(s.importURL))
	mux.HandleFunc("POST /api/credits/parse", s.member(s.parseCredits))
	mux.HandleFunc("GET /api/sources/search", s.member(s.searchSources))
	mux.HandleFunc("GET /api/wikidata/search", s.member(s.searchWikidata))
	mux.HandleFunc("PUT /api/people/{id}/wikidata", s.member(s.linkWikidata))
	mux.HandleFunc("POST /api/productions", s.member(s.createProduction))
	mux.HandleFunc("GET /api/productions/{id}", s.member(s.getProduction))
	mux.HandleFunc("PUT /api/productions/{id}", s.member(s.updateProduction))
	mux.HandleFunc("GET /api/people/{id}", s.member(s.getPerson))
	mux.HandleFunc("PUT /api/people/{id}", s.member(s.updatePerson))
	mux.HandleFunc("GET /api/venues/{id}", s.member(s.getVenue))
	mux.HandleFunc("PUT /api/venues/{id}", s.member(s.updateVenue))
	mux.HandleFunc("GET /api/images/{id}", s.member(s.getImage))

	mux.HandleFunc("GET /api/outings", s.member(s.listOutings))
	mux.HandleFunc("POST /api/outings", s.member(s.createOuting))
	mux.HandleFunc("PUT /api/outings/{id}", s.member(s.updateOuting))
	mux.HandleFunc("DELETE /api/outings/{id}", s.member(s.deleteOuting))
	mux.HandleFunc("GET /api/favorites", s.member(s.listFavorites))
	mux.HandleFunc("PUT /api/favorites/{entity}/{id}", s.member(s.putFavorite))
	mux.HandleFunc("DELETE /api/favorites/{entity}/{id}", s.member(s.deleteFavorite))
	mux.HandleFunc("PUT /api/notes/{entity}/{id}", s.member(s.putNote))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Introuvable.")
	})
	mux.HandleFunc("/", s.frontend)

	csrf := http.NewCrossOriginProtection()
	s.handler = securityHeaders(csrf.Handler(s.session(mux)))
	return s
}

const sessionCookie = "strapontin_session"

type ctxKey struct{}

// session attaches the signed-in member, if any, to the request context.
func (s *Server) session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			m, err := auth.LookupSession(s.db, c.Value)
			if err != nil {
				log.Printf("session lookup: %v", err)
			}
			if m != nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, m))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func currentMember(r *http.Request) *auth.Member {
	m, _ := r.Context().Value(ctxKey{}).(*auth.Member)
	return m
}

type memberHandler func(w http.ResponseWriter, r *http.Request, m *auth.Member)

// member rejects requests without a session.
func (s *Server) member(h memberHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := currentMember(r)
		if m == nil {
			writeError(w, http.StatusUnauthorized, "Connectez-vous pour continuer.")
			return
		}
		h(w, r, m)
	}
}

// admin rejects requests from members without the admin role.
func (s *Server) admin(h memberHandler) http.HandlerFunc {
	return s.member(func(w http.ResponseWriter, r *http.Request, m *auth.Member) {
		if !m.IsAdmin() {
			writeError(w, http.StatusForbidden, "Réservé à l'administration.")
			return
		}
		h(w, r, m)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

// clientIP reads X-Forwarded-For only behind a trusted proxy, which appends
// the real client address as the last entry.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// maxBody caps JSON request bodies.
const maxBody = 1 << 20

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Requête invalide.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError sends a message meant to be shown to the member as is.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// serverError logs err and hides it from the client.
func serverError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "Erreur du serveur.")
}

// notFoundOr reports sql.ErrNoRows as 404 and anything else as a server error.
func notFoundOr(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	serverError(w, err)
}

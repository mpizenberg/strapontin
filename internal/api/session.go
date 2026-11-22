package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/mpizenberg/strapontin/internal/auth"
)

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		// Plain-HTTP local development would otherwise drop the cookie in some browsers.
		Secure:   !strings.HasPrefix(s.cfg.BaseURL, "http://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// startSession signs m in on this browser.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, m *auth.Member) bool {
	token, err := auth.CreateSession(s.db, m.ID, r.UserAgent())
	if err != nil {
		serverError(w, err)
		return false
	}
	s.setSessionCookie(w, token, auth.SessionLifetime)
	return true
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Login, Password string }
	if !decode(w, r, &req) {
		return
	}
	login := strings.TrimSpace(strings.ToLower(req.Login))
	if !s.logins.Allow("ip:"+s.clientIP(r), "login:"+login) {
		writeError(w, http.StatusTooManyRequests, "Trop de tentatives. Réessayez dans un quart d'heure.")
		return
	}
	m, err := auth.CheckLogin(s.db, login, req.Password)
	if err != nil {
		serverError(w, err)
		return
	}
	if m == nil {
		auth.LogEvent(s.db, nil, "login_failed")
		writeError(w, http.StatusUnauthorized, "Identifiant ou mot de passe incorrect.")
		return
	}
	if !s.startSession(w, r, m) {
		return
	}
	auth.LogEvent(s.db, &m.ID, "login")
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		auth.DeleteSession(s.db, c.Value)
	}
	s.setSessionCookie(w, "", -time.Second)
	w.WriteHeader(http.StatusNoContent)
}

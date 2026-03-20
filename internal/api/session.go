package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type session struct {
	createdAt time.Time
}

type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*session
	maxAge   time.Duration
}

func newSessionStore() *sessionStore {
	s := &sessionStore{
		sessions: make(map[string]*session),
		maxAge:   24 * time.Hour,
	}
	go s.cleanup()
	return s
}

func (s *sessionStore) create() string {
	b := make([]byte, 32)
	rand.Read(b)
	id := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[id] = &session{createdAt: time.Now()}
	s.mu.Unlock()
	return id
}

func (s *sessionStore) valid(id string) bool {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return time.Since(sess.createdAt) < s.maxAge
}

func (s *sessionStore) remove(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *sessionStore) cleanup() {
	ticker := time.NewTicker(time.Hour)
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if time.Since(sess.createdAt) >= s.maxAge {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

const sessionCookieName = "airpool_session"

func setSessionCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

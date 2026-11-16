package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookie = "JSESSIONID"

// Session replaces HttpSession. UserID is the legacy "uidBean" attribute and CreationDate the
// "sessionCreationDate" attribute. Values holds anything else a feature wants to keep.
type Session struct {
	ID           string
	UserID       string
	CreationDate time.Time
	Values       map[string]any
	lastSeen     time.Time
}

// SessionStore keeps sessions in memory with the legacy 30 minute idle timeout. With more than
// one ECS task the ALB target group needs stickiness (see docs/MIGRATION_PLAN.md).
type SessionStore struct {
	mutex    sync.Mutex
	sessions map[string]*Session
	path     string
	timeout  time.Duration
}

func NewSessionStore(cookiePath string) *SessionStore {
	return &SessionStore{sessions: map[string]*Session{}, path: cookiePath, timeout: 30 * time.Minute}
}

// Get returns the request's session, creating one (and setting the cookie) when there is none.
func (s *SessionStore) Get(w http.ResponseWriter, r *http.Request) *Session {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	now := time.Now()
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if session, ok := s.sessions[cookie.Value]; ok {
			if now.Sub(session.lastSeen) < s.timeout {
				session.lastSeen = now
				return session
			}
			delete(s.sessions, cookie.Value)
		}
	}
	for id, session := range s.sessions {
		if now.Sub(session.lastSeen) >= s.timeout {
			delete(s.sessions, id)
		}
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	session := &Session{ID: hex.EncodeToString(raw), Values: map[string]any{}, lastSeen: now}
	s.sessions[session.ID] = session
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.ID, Path: s.path, HttpOnly: true})
	return session
}

// Invalidate is HttpSession.invalidate(): drops the session and expires the cookie.
func (s *SessionStore) Invalidate(w http.ResponseWriter, session *Session) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.sessions, session.ID)
	session.UserID = ""
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: s.path, MaxAge: -1, HttpOnly: true})
}

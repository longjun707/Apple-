package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"apple-hme-manager/internal/apple"

	"github.com/gin-gonic/gin"
)

// Server holds the API server state
type Server struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
}

// Session TTL constants
const (
	DefaultSessionTTL    = 2 * time.Hour
	RememberMeSessionTTL = 7 * 24 * time.Hour
	SessionCleanInterval = 10 * time.Minute
)

// SessionState holds per-session state
type SessionState struct {
	Auth       *apple.AppleAuth
	HME        *apple.HMEClient
	AccountID  uint   // Database account ID (Apple account)
	AppleID    string // Apple ID email
	AdminID    uint   // Admin user ID
	AdminName  string // Admin username
	RememberMe bool   // Whether to keep session longer
	CreatedAt  time.Time
	LastActive time.Time
}

// NewServer creates a new API server
func NewServer() *Server {
	s := &Server{
		sessions: make(map[string]*SessionState),
	}
	go s.cleanExpiredSessions()
	return s
}

// cleanExpiredSessions periodically removes expired sessions
func (s *Server) cleanExpiredSessions() {
	ticker := time.NewTicker(SessionCleanInterval)
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, sess := range s.sessions {
			ttl := DefaultSessionTTL
			if sess.RememberMe {
				ttl = RememberMeSessionTTL
			}
			if now.Sub(sess.LastActive) > ttl {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// getSession retrieves or creates a session, checking expiry
func (s *Server) getSession(sessionID string) *SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[sessionID]; ok {
		ttl := DefaultSessionTTL
		if session.RememberMe {
			ttl = RememberMeSessionTTL
		}
		if time.Since(session.LastActive) > ttl {
			delete(s.sessions, sessionID)
		} else {
			session.LastActive = time.Now()
			return session
		}
	}

	now := time.Now()
	session := &SessionState{
		CreatedAt:  now,
		LastActive: now,
	}
	s.sessions[sessionID] = session
	return session
}

// Request/Response types
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type TwoFARequest struct {
	Code    string `json:"code" binding:"required"`
	PhoneID int    `json:"phoneId,omitempty"`
	Method  string `json:"method,omitempty"` // "device" or "sms"
}

type CreateHMERequest struct {
	Label          string `json:"label,omitempty"`
	Note           string `json:"note,omitempty"`
	ForwardToEmail string `json:"forwardToEmail,omitempty"`
}

type BatchCreateRequest struct {
	Count          int    `json:"count" binding:"required,min=1,max=100"`
	LabelPrefix    string `json:"labelPrefix,omitempty"`
	DelayMs        int    `json:"delayMs,omitempty"`
	ForwardToEmail string `json:"forwardToEmail,omitempty"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SessionMiddleware extracts or generates session ID
func (s *Server) SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.GetHeader("X-Session-ID")
		if sessionID == "" {
			sessionID = c.Query("session")
		}
		if sessionID == "" {
			sessionID = generateSessionID()
		}
		c.Set("sessionID", sessionID)
		c.Set("session", s.getSession(sessionID))
		c.Header("X-Session-ID", sessionID)
		c.Next()
	}
}

// AdminAuthMiddleware checks that the session belongs to an authenticated admin
func (s *Server) AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := c.MustGet("session").(*SessionState)
		if session.AdminID == 0 {
			c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "未登录"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Health check
func (s *Server) Health(c *gin.Context) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

// generateSessionID creates a cryptographically random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen, but just in case
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

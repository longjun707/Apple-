package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"apple-hme-manager/internal/apple"
	"apple-hme-manager/internal/store"

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

// Login handler
func (s *Server) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	session := c.MustGet("session").(*SessionState)
	result, err := session.Auth.Login(req.Username, req.Password)

	// Log to database
	if store.DB != nil {
		acctRepo := store.NewAccountRepo()
		account, _ := acctRepo.FindOrCreate(req.Username)
		if account != nil {
			session.AccountID = account.ID
			session.AppleID = req.Username

			// Save login log
			logRepo := store.NewLoginLogRepo()
			status := "success"
			msg := ""
			if err != nil {
				status = "failed"
				msg = err.Error()
			} else if result != nil && result.Requires2FA {
				status = "2fa_required"
			}
			logRepo.Create(&store.LoginLog{
				AccountID: account.ID,
				IP:        c.ClientIP(),
				UserAgent: c.GetHeader("User-Agent"),
				Status:    status,
				Message:   msg,
			})

			if status == "success" {
				acctRepo.UpdateLastLogin(account.ID)
			}
		}
	}

	if err != nil {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: result})
}

// Verify2FA handler
func (s *Server) Verify2FA(c *gin.Context) {
	var req TwoFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	session := c.MustGet("session").(*SessionState)

	var err error
	if req.Method == "sms" {
		err = session.Auth.Verify2FASMS(req.PhoneID, req.Code)
	} else {
		err = session.Auth.Verify2FADevice(req.Code)
	}

	if err != nil {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]bool{"authenticated": true}})
}

// RequestSMS handler
func (s *Server) RequestSMS(c *gin.Context) {
	var req struct {
		PhoneID int `json:"phoneId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	session := c.MustGet("session").(*SessionState)
	if err := session.Auth.RequestSMSCode(req.PhoneID); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// GetAccount handler
func (s *Server) GetAccount(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	info, err := session.HME.GetAccountInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: info})
}

// ListHME handler
func (s *Server) ListHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	emails, err := session.HME.ListEmails()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
}

// CreateHME handler
func (s *Server) CreateHME(c *gin.Context) {
	var req CreateHMERequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	hme, err := session.HME.CreateEmail(req.Label, req.Note, req.ForwardToEmail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Save to database
	if store.DB != nil && session.AccountID > 0 {
		hmeRepo := store.NewHMERepo()
		hmeRepo.Create(&store.HMERecord{
			AccountID:      session.AccountID,
			HMEID:          hme.ID,
			EmailAddress:   hme.EmailAddress,
			Label:          hme.Label,
			Note:           hme.Note,
			ForwardToEmail: hme.ForwardToEmail,
			Active:         hme.Active,
		})
		// Update account HME count
		count, _ := hmeRepo.Count(session.AccountID)
		store.NewAccountRepo().UpdateHMECount(session.AccountID, int(count))
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: hme})
}

// BatchCreateHME handler
func (s *Server) BatchCreateHME(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	if req.LabelPrefix == "" {
		req.LabelPrefix = "Auto"
	}
	if req.DelayMs == 0 {
		req.DelayMs = 1000
	}

	results, errors := session.HME.BatchCreateEmails(req.Count, req.LabelPrefix, req.DelayMs, req.ForwardToEmail)

	errorStrings := make([]string, len(errors))
	for i, err := range errors {
		errorStrings[i] = err.Error()
	}

	// Save to database
	if store.DB != nil && session.AccountID > 0 && len(results) > 0 {
		hmeRepo := store.NewHMERepo()
		records := make([]store.HMERecord, len(results))
		for i, hme := range results {
			records[i] = store.HMERecord{
				AccountID:      session.AccountID,
				HMEID:          hme.ID,
				EmailAddress:   hme.EmailAddress,
				Label:          hme.Label,
				Note:           hme.Note,
				ForwardToEmail: hme.ForwardToEmail,
				Active:         hme.Active,
			}
		}
		hmeRepo.BatchCreate(records)
		// Update account HME count
		count, _ := hmeRepo.Count(session.AccountID)
		store.NewAccountRepo().UpdateHMECount(session.AccountID, int(count))
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"created": results,
			"errors":  errorStrings,
			"total":   req.Count,
			"success": len(results),
			"failed":  len(errors),
		},
	})
}

// DeleteHME handler
func (s *Server) DeleteHME(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "id required"})
		return
	}

	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	if err := session.HME.DeleteEmail(id); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// GetForwardEmails handler
func (s *Server) GetForwardEmails(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	if !session.Auth.IsAuthenticated() {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "not authenticated"})
		return
	}

	emails, err := session.HME.GetForwardEmails()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
}

// Logout handler
func (s *Server) Logout(c *gin.Context) {
	sessionID := c.MustGet("sessionID").(string)

	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	c.JSON(http.StatusOK, APIResponse{Success: true})
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

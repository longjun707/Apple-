package apple

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

// API endpoints
const (
	AuthBase      = "https://idmsa.apple.com/appleauth/auth"
	AccountBase   = "https://appleid.apple.com/account/manage"
	BootstrapURL  = "https://account.apple.com/bootstrap/portal"
	TokenURL      = "https://appleid.apple.com/account/manage/gs/ws/token"
	AuthWidgetKey = "af1139274f266b22b68c2a3e7ad932cb3c0bbe854e13a79af78dcc73136882c3"
	HMEWidgetKey  = "cbf64fd6843ee630b463f358ea0b707b"
)

// Browser simulation
const (
	ChromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

// Session holds the authenticated session state
type Session struct {
	mu           sync.RWMutex
	Client       *http.Client
	Cookies      []*http.Cookie
	SCNT         string
	SessionID    string
	SessionToken string
	AppleID      string
	Authenticated bool
	LastActivity time.Time
}

// NewSession creates a new session
func NewSession() *Session {
	jar, _ := cookiejar.New(nil)
	return &Session{
		Client: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		LastActivity: time.Now(),
	}
}

// SRPInitRequest represents the init request
type SRPInitRequest struct {
	A           string   `json:"a"`
	AccountName string   `json:"accountName"`
	Protocols   []string `json:"protocols"`
}

// SRPInitResponse represents the init response
type SRPInitResponse struct {
	Protocol  string `json:"protocol"`
	Salt      string `json:"salt"`
	Iteration int    `json:"iteration"`
	B         string `json:"b"`
	C         string `json:"c"`
}

// SRPCompleteRequest represents the complete request
type SRPCompleteRequest struct {
	AccountName string   `json:"accountName"`
	TrustTokens []string `json:"trustTokens"`
	RememberMe  bool     `json:"rememberMe"`
	M1          string   `json:"m1"`
	M2          string   `json:"m2"`
	C           string   `json:"c"`
}

// TwoFARequest represents a 2FA verification request
type TwoFARequest struct {
	SecurityCode SecurityCode `json:"securityCode"`
}

// SecurityCode represents the security code structure
type SecurityCode struct {
	Code string `json:"code"`
}

// TwoFASMSRequest represents SMS 2FA request
type TwoFASMSRequest struct {
	SecurityCode SecurityCode `json:"securityCode"`
	PhoneNumber  PhoneNumber  `json:"phoneNumber"`
	Mode         string       `json:"mode"`
}

// PhoneNumber represents phone info
type PhoneNumber struct {
	ID int `json:"id"`
}

// AccountInfo represents account information
type AccountInfo struct {
	AppleID                 string   `json:"appleId"`
	AlternateEmailAddresses []string `json:"alternateEmailAddresses,omitempty"`
	AddAlternateEmail       struct {
		Pending bool `json:"pending"`
		Vetted  bool `json:"vetted"`
	} `json:"addAlternateEmail,omitempty"`
}

// HMEEmail represents a Hide My Email address
type HMEEmail struct {
	ID             string `json:"id"`
	EmailAddress   string `json:"emailAddress"`
	Label          string `json:"label"`
	Note           string `json:"note"`
	ForwardToEmail string `json:"forwardToEmail"`
	Active         bool   `json:"active"`
	CreateTime     int64  `json:"createTime"`
	Type           string `json:"type"`
}

// HMEListResponse represents HME list response
type HMEListResponse struct {
	PrivateEmailList []HMEEmail `json:"privateEmailList"`
}

// HMECreateRequest represents HME creation request
type HMECreateRequest struct {
	EmailAddress   string `json:"emailAddress"`
	Label          string `json:"label"`
	Note           string `json:"note"`
	ForwardToEmail string `json:"forwardToEmail,omitempty"`
}

// ForwardEmailResponse represents forward email options
type ForwardEmailResponse struct {
	ForwardToOptions struct {
		AvailableEmails []string `json:"availableEmails"`
	} `json:"forwardToOptions"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	ServiceErrors []ServiceError `json:"serviceErrors,omitempty"`
	Error         string         `json:"error,omitempty"`
}

// ServiceError represents a service error
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AuthState represents the current auth state
type AuthState int

const (
	AuthStateNone AuthState = iota
	AuthStateNeedsPassword
	AuthStateNeeds2FA
	AuthStateAuthenticated
)

// LoginResult represents login outcome
type LoginResult struct {
	State        AuthState `json:"state"`
	Message      string    `json:"message,omitempty"`
	SessionID    string    `json:"sessionId,omitempty"`
	Requires2FA  bool      `json:"requires2fa,omitempty"`
	PhoneNumbers []struct {
		ID          int    `json:"id"`
		NumberWithDialCode string `json:"numberWithDialCode"`
	} `json:"phoneNumbers,omitempty"`
}

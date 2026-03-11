package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"apple-hme-manager/internal/apple"
	"apple-hme-manager/internal/store"

	"github.com/gin-gonic/gin"
)

// AdminLoginRequest represents admin login request
type AdminLoginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"rememberMe"`
}

// AccountRequest represents account create request
type AccountRequest struct {
	AppleID  string `json:"appleId" binding:"required"`
	Password string `json:"password" binding:"required"`
	Remark   string `json:"remark"`
}

// UpdateAccountRequest represents account update request (password is optional)
type UpdateAccountRequest struct {
	AppleID  string `json:"appleId" binding:"required"`
	Password string `json:"password"` // 留空则不修改
	Remark   string `json:"remark"`
}

// BatchImportRequest represents a batch account import request
type BatchImportRequest struct {
	Accounts []AccountRequest `json:"accounts" binding:"required,min=1,max=500"`
}

// BatchImportResult represents the result of a batch account import
type BatchImportResult struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors"`
}

// clampPageSize enforces pageSize bounds [1, 100], defaulting to 20.
func clampPageSize(pageSize int) int {
	if pageSize < 1 {
		return 20
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

// escapeLike escapes SQL LIKE wildcards and the escape character itself.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// saveAppleSession persists Apple auth session (tokens + cookies) to database
func saveAppleSession(accountID uint, auth *apple.AppleAuth) {
	if store.DB == nil || auth == nil {
		return
	}
	token, scnt, sessionID, cookies := auth.ExportSessionData()
	if token == "" {
		return
	}
	store.NewAccountRepo().SaveSession(accountID, token, scnt, sessionID, cookies)
	log.Printf("[Session] Saved Apple session for account %d (cookies_len=%d)", accountID, len(cookies))
}

// fetchAndSaveAccountInfo fetches family sharing and profile info and saves to database
func fetchAndSaveAccountInfo(accountID uint, hme *apple.HMEClient) {
	if store.DB == nil || hme == nil {
		return
	}

	updates := map[string]interface{}{}

	// 1. Get profile info (birthday, country, devices, phone numbers)
	profileResp, err := hme.GetAccountProfile()
	if err != nil {
		log.Printf("[Profile] Failed to get profile for account %d: %v", accountID, err)
	} else {
		if profileResp.Birthday != "" {
			updates["birthday"] = profileResp.Birthday
		}
		if profileResp.Country != "" {
			updates["country"] = profileResp.Country
		}
		if len(profileResp.AlternateEmails) > 0 {
			emailsJSON, _ := json.Marshal(profileResp.AlternateEmails)
			updates["alternate_emails"] = string(emailsJSON)
		}
		// Save phone numbers from security section
		if len(profileResp.PhoneNumbers) > 0 {
			phonesJSON, _ := json.Marshal(profileResp.PhoneNumbers)
			updates["phone_numbers"] = string(phonesJSON)
			log.Printf("[Profile] Saving %d phone numbers for account %d", len(profileResp.PhoneNumbers), accountID)
		}
		updates["trusted_device_count"] = profileResp.TrustedDeviceCount
	}

	// 2. Get family info and fullName (optional, may fail for restored sessions)
	familyResp, err := hme.GetFamilyMembers()
	if err != nil {
		// Family API often fails for restored sessions due to iframe cookie requirements
		// This is non-critical, so we just skip it silently
	} else {
		updates["family_member_count"] = len(familyResp.FamilyMembers)

		// Find current user and get fullName + role
		for _, m := range familyResp.FamilyMembers {
			if m.Dsid == familyResp.CurrentDsid {
				// Get fullName from family member
				if m.FullName != "" {
					updates["full_name"] = m.FullName
				}
				// Determine role
				if familyResp.Family != nil && familyResp.Family.OrganizerDsid == m.Dsid {
					updates["is_family_organizer"] = true
					updates["family_role"] = "organizer"
				} else if m.IsParent {
					updates["is_family_organizer"] = false
					updates["family_role"] = "parent"
				} else if m.AgeClassification == "CHILD" {
					updates["is_family_organizer"] = false
					updates["family_role"] = "child"
				} else {
					updates["is_family_organizer"] = false
					updates["family_role"] = "adult"
				}
				break
			}
		}
	}

	if len(updates) > 0 {
		result := store.DB.Model(&store.Account{}).Where("id = ?", accountID).Updates(updates)
		if result.Error != nil {
			log.Printf("[Account] ERROR saving account info for %d: %v", accountID, result.Error)
		} else {
			log.Printf("[Account] Saved account info for %d (rows=%d): %+v", accountID, result.RowsAffected, updates)
		}
	} else {
		log.Printf("[Account] No updates to save for account %d", accountID)
	}
}

// maxSessionAge is the maximum age for a restored Apple session (7 days)
const maxSessionAge = 7 * 24 * time.Hour

// ensureAppleSession checks if Apple session is active for the account, tries to restore from DB if not
func ensureAppleSession(session *SessionState, accountID uint) bool {
	if accountID == 0 {
		return false
	}
	// Already active in memory
	if session.HME != nil && session.AccountID == accountID && session.Auth != nil && session.Auth.IsAuthenticated() {
		return true
	}
	// Try restore from database
	if store.DB == nil {
		return false
	}
	account, err := store.NewAccountRepo().FindByID(accountID)
	if err != nil || account.SessionToken == "" {
		return false
	}
	// Reject stale sessions
	if account.SessionSavedAt == nil || time.Since(*account.SessionSavedAt) > maxSessionAge {
		log.Printf("[Session] Session for account %d is too old, skipping restore", accountID)
		return false
	}
	log.Printf("[Session] Restoring session for account %d...", accountID)
	auth := apple.RestoreAppleAuth(account.SessionToken, account.SessionSCNT, account.SessionID, account.SessionCookies)
	hme := apple.NewHMEClient(auth)

	// Mark session as restored - skip Bootstrap validation, let HME API validate directly
	hme.MarkRestoredSession()

	session.Auth = auth
	session.HME = hme
	session.AccountID = accountID
	session.AppleID = account.AppleID
	log.Printf("[Session] Restored Apple session for account %d from DB (skipping validation)", accountID)
	return true
}

// AdminLogin handles admin login
func (s *Server) AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	adminRepo := store.NewAdminRepo()
	admin, err := adminRepo.FindByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "用户名或密码错误"})
		return
	}

	if !admin.CheckPassword(req.Password) {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "用户名或密码错误"})
		return
	}

	// Update last login
	adminRepo.UpdateLastLogin(admin.ID)

	// Store admin info in session
	session := c.MustGet("session").(*SessionState)
	session.AdminID = admin.ID
	session.AdminName = admin.Username
	session.RememberMe = req.RememberMe

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"id":       admin.ID,
			"username": admin.Username,
			"nickname": admin.Nickname,
			"role":     admin.Role,
		},
	})
}

// AdminLogout handles admin logout
func (s *Server) AdminLogout(c *gin.Context) {
	sessionID := c.MustGet("sessionID").(string)
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// AdminInfo returns current admin info
func (s *Server) AdminInfo(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)
	data := map[string]interface{}{
		"id":       session.AdminID,
		"username": session.AdminName,
	}
	// Fetch full admin details from DB so frontend gets nickname/role
	if store.DB != nil {
		if admin, err := store.NewAdminRepo().FindByID(session.AdminID); err == nil {
			data["nickname"] = admin.Nickname
			data["role"] = admin.Role
		}
	}
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data})
}

// ListAccounts returns all Apple accounts
func (s *Server) ListAccounts(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	pageSize = clampPageSize(pageSize)
	search := escapeLike(c.Query("search"))

	accounts, total, err := store.NewAccountRepo().List(page, pageSize, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"list":     accounts,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// BatchCreateAccounts imports multiple Apple accounts at once
func (s *Server) BatchCreateAccounts(c *gin.Context) {
	var req BatchImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "参数无效: " + err.Error()})
		return
	}

	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	result := BatchImportResult{
		Errors: make([]string, 0),
	}

	for _, item := range req.Accounts {
		if item.AppleID == "" || item.Password == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: Apple ID 或密码为空", item.AppleID))
			continue
		}

		// Check if account already exists
		var existing store.Account
		if err := store.DB.Where("apple_id = ?", item.AppleID).First(&existing).Error; err == nil {
			result.Skipped++
			continue
		}

		account := &store.Account{
			AppleID:  item.AppleID,
			Password: store.EncryptPassword(item.Password),
			Remark:   item.Remark,
			Status:   1,
		}

		if err := store.DB.Create(account).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.AppleID, err))
			continue
		}

		result.Created++
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: result})
}

// CreateAccount creates a new Apple account
func (s *Server) CreateAccount(c *gin.Context) {
	var req AccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	// Check if account already exists
	var existing store.Account
	if err := store.DB.Where("apple_id = ?", req.AppleID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "该 Apple ID 已存在"})
		return
	}

	account := &store.Account{
		AppleID:  req.AppleID,
		Password: store.EncryptPassword(req.Password),
		Remark:   req.Remark,
		Status:   1,
	}

	if err := store.DB.Create(account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: account})
}

// UpdateAccount updates an Apple account
func (s *Server) UpdateAccount(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Check for duplicate Apple ID (exclude current account)
	var existing store.Account
	if err := store.DB.Where("apple_id = ? AND id != ?", req.AppleID, id).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "该 Apple ID 已被其他账户使用"})
		return
	}

	updates := map[string]interface{}{
		"apple_id": req.AppleID,
		"remark":   req.Remark,
	}
	if req.Password != "" {
		updates["password"] = store.EncryptPassword(req.Password)
	}

	if err := store.DB.Model(&store.Account{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// DeleteAccount deletes an Apple account and all related data
func (s *Server) DeleteAccount(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if err := store.NewAccountRepo().DeleteCascade(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// LoginAppleAccount logs into an Apple account and fetches HME
func (s *Server) LoginAppleAccount(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	// Get account from database
	var account store.Account
	if err := store.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, APIResponse{Success: false, Error: "账户不存在"})
		return
	}

	// Decrypt password for Apple login
	plainPassword, decErr := store.DecryptPassword(account.Password)
	if decErr != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "密码解密失败: " + decErr.Error()})
		return
	}

	// Create Apple auth client
	auth := apple.NewAppleAuth()
	result, err := auth.Login(account.AppleID, plainPassword)

	// Update account status
	now := time.Now()
	updates := map[string]interface{}{
		"last_login": now,
	}

	if err != nil {
		updates["last_error"] = err.Error()
		updates["status"] = 2 // locked/error
		store.DB.Model(&account).Updates(updates)
		c.JSON(http.StatusOK, APIResponse{
			Success: false,
			Error:   err.Error(),
			Data:    map[string]interface{}{"requires2fa": false},
		})
		return
	}

	if result.Requires2FA {
		updates["last_error"] = "需要2FA验证"
		
		// Save phone numbers to database (from 2FA trusted phones)
		if len(result.PhoneNumbers) > 0 {
			phonesJSON, _ := json.Marshal(result.PhoneNumbers)
			updates["phone_numbers"] = string(phonesJSON)
			log.Printf("[Login] Saving %d trusted phone numbers for account %d: %s", len(result.PhoneNumbers), id, string(phonesJSON))
		} else {
			log.Printf("[Login] No trusted phone numbers returned for account %d", id)
		}
		
		store.DB.Model(&account).Updates(updates)

		// Store auth client for 2FA
		session.Auth = auth
		session.HME = apple.NewHMEClient(auth)
		session.AccountID = uint(id)
		session.AppleID = account.AppleID

		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"requires2fa":  true,
				"message":      "需要2FA验证",
				"phoneNumbers": result.PhoneNumbers,
			},
		})
		return
	}

	// Login successful
	updates["last_error"] = ""
	updates["status"] = 1
	store.DB.Model(&account).Updates(updates)

	// Store auth for HME operations
	session.Auth = auth
	session.HME = apple.NewHMEClient(auth)
	session.AccountID = uint(id)
	session.AppleID = account.AppleID

	// Persist session to database — export session data before goroutine to avoid race
	token, scnt, sessID, cookies := auth.ExportSessionData()
	go func() {
		if store.DB != nil && token != "" {
			store.NewAccountRepo().SaveSession(uint(id), token, scnt, sessID, cookies)
		}
	}()

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"requires2fa": false,
			"message":     "登录成功",
		},
	})
}

// Verify2FAForAccount verifies 2FA for an Apple account
func (s *Server) Verify2FAForAccount(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	// Validate URL :id matches session's current account
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}
	if session.Auth == nil || session.AccountID != uint(id) {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req TwoFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	var err error
	if req.Method == "sms" {
		err = session.Auth.Verify2FASMS(req.PhoneID, req.Code)
	} else {
		err = session.Auth.Verify2FADevice(req.Code)
	}

	if err != nil {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Log cookies immediately after 2FA (before Bootstrap)
	log.Printf("[2FA] Cookies BEFORE Bootstrap:")
	session.Auth.LogAllCookies()

	// Create HME client and run Bootstrap to get all necessary cookies (aidsp, idclient, etc.)
	if session.HME == nil {
		session.HME = apple.NewHMEClient(session.Auth)
	}
	if err := session.HME.Bootstrap(); err != nil {
		log.Printf("[2FA] Bootstrap warning (session still valid): %v", err)
	}

	// Log cookies after Bootstrap
	log.Printf("[2FA] Cookies AFTER Bootstrap:")
	session.Auth.LogAllCookies()

	// Update account status
	if store.DB != nil {
		store.DB.Model(&store.Account{}).Where("id = ?", session.AccountID).Updates(map[string]interface{}{
			"status":             1,
			"last_error":         "",
			"two_factor_enabled": true,
		})
	}

	// Persist full Apple session (tokens + cookies) to database AFTER Bootstrap
	saveAppleSession(session.AccountID, session.Auth)

	// Fetch and save account info (profile + family) - run synchronously so data is ready for frontend
	fetchAndSaveAccountInfo(session.AccountID, session.HME)

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]bool{"authenticated": true}})
}

// RequestSMSForAccount requests SMS 2FA code for an Apple account
func (s *Server) RequestSMSForAccount(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)
	if session.Auth == nil {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "请先登录Apple账户"})
		return
	}

	var req struct {
		PhoneID int `json:"phoneId"`
	}
	c.ShouldBindJSON(&req)
	if req.PhoneID == 0 {
		req.PhoneID = 1
	}

	if err := session.Auth.RequestSMSCode(req.PhoneID); err != nil {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]string{"message": "短信已发送"}})
}

// clearInvalidSession clears an invalid session from database
func clearInvalidSession(accountID uint) {
	if store.DB == nil {
		return
	}
	store.DB.Model(&store.Account{}).Where("id = ?", accountID).Updates(map[string]interface{}{
		"session_token":    "",
		"session_scnt":     "",
		"session_id":       "",
		"session_cookies":  "",
		"session_saved_at": nil,
	})
	log.Printf("[Session] Cleared invalid session for account %d", accountID)
}

// RefreshAllSessions extends all saved sessions
// This is called when the server starts and periodically thereafter
func RefreshAllSessions() {
	if store.DB == nil {
		log.Printf("[Session] Database not connected, skipping session refresh")
		return
	}

	log.Printf("[Session] Starting to refresh all saved sessions...")

	// Find all accounts with saved sessions
	var accounts []store.Account
	if err := store.DB.Where("session_token != '' AND session_token IS NOT NULL").Find(&accounts).Error; err != nil {
		log.Printf("[Session] Failed to query accounts: %v", err)
		return
	}

	if len(accounts) == 0 {
		log.Printf("[Session] No saved sessions found")
		return
	}

	log.Printf("[Session] Found %d accounts with saved sessions", len(accounts))

	valid := 0
	invalid := 0

	for _, account := range accounts {
		log.Printf("[Session] Refreshing session for account %d (%s)...", account.ID, account.AppleID)

		// Restore session
		auth := apple.RestoreAppleAuth(account.SessionToken, account.SessionSCNT, account.SessionID, account.SessionCookies)
		hme := apple.NewHMEClient(auth)

		// Try to extend session
		ok, err := hme.ExtendSession()
		if ok {
			log.Printf("[Session] ✓ Account %d session extended successfully", account.ID)
			// Update saved session (new cookies may have been set)
			saveAppleSession(account.ID, auth)
			valid++
		} else {
			log.Printf("[Session] ✗ Account %d session invalid: %v", account.ID, err)
			// Don't clear - let user re-login manually
			invalid++
		}
	}

	log.Printf("[Session] Session refresh complete: %d valid, %d invalid (of %d total)", valid, invalid, len(accounts))
}

// StartPeriodicSessionRefresh starts a goroutine that refreshes sessions every 3-5 minutes (random)
func StartPeriodicSessionRefresh() {
	// Initial refresh on startup
	RefreshAllSessions()

	// Then refresh with random interval between 3-5 minutes
	go func() {
		for {
			// Random delay: 3-5 minutes (180-300 seconds)
			delaySec := 180 + time.Duration(time.Now().UnixNano()%120)
			log.Printf("[Session] Next refresh in %d seconds", delaySec)
			time.Sleep(delaySec * time.Second)

			log.Printf("[Session] Periodic session refresh triggered")
			RefreshAllSessions()
		}
	}()
	log.Printf("[Session] Periodic session refresh started (random 3-5 minutes)")
}

// AutoHMETaskStatus holds the current status of auto HME creation task
type AutoHMETaskStatus struct {
	mu                sync.RWMutex
	Enabled           bool      `json:"enabled"`
	Running           bool      `json:"running"`
	IntervalMinutes   int       `json:"intervalMinutes"`
	CountPerAccount   int       `json:"countPerAccount"`
	LastRunTime       *time.Time `json:"lastRunTime"`
	NextRunTime       *time.Time `json:"nextRunTime"`
	CurrentAccount    string    `json:"currentAccount"`
	CurrentProgress   int       `json:"currentProgress"`
	TotalAccounts     int       `json:"totalAccounts"`
	ProcessedAccounts int       `json:"processedAccounts"`
	TotalCreated      int       `json:"totalCreated"`
	TotalFailed       int       `json:"totalFailed"`
	Logs              []AutoHMELogEntry `json:"logs"`
}

// AutoHMELogEntry represents a single log entry
type AutoHMELogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // info, success, error, warning
	Message string    `json:"message"`
}

var autoHMEStatus = &AutoHMETaskStatus{
	Enabled:         true,
	IntervalMinutes: 30,
	CountPerAccount: 20,
	Logs:            make([]AutoHMELogEntry, 0),
}

const maxLogEntries = 500

// addAutoHMELog adds a log entry (thread-safe)
func addAutoHMELog(level, message string) {
	autoHMEStatus.mu.Lock()
	defer autoHMEStatus.mu.Unlock()
	
	entry := AutoHMELogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	}
	autoHMEStatus.Logs = append(autoHMEStatus.Logs, entry)
	
	// Keep only last N entries
	if len(autoHMEStatus.Logs) > maxLogEntries {
		autoHMEStatus.Logs = autoHMEStatus.Logs[len(autoHMEStatus.Logs)-maxLogEntries:]
	}
	
	log.Printf("[AutoHME] %s", message)
}

// StartPeriodicHMECreation starts a goroutine that creates HME for all accounts periodically
func StartPeriodicHMECreation() {
	go func() {
		for {
			// Read settings under lock
			autoHMEStatus.mu.RLock()
			intervalMin := autoHMEStatus.IntervalMinutes
			autoHMEStatus.mu.RUnlock()

			nextRun := time.Now().Add(time.Duration(intervalMin) * time.Minute)
			autoHMEStatus.mu.Lock()
			autoHMEStatus.NextRunTime = &nextRun
			autoHMEStatus.mu.Unlock()

			addAutoHMELog("info", fmt.Sprintf("下次自动创建将在 %d 分钟后执行", intervalMin))
			time.Sleep(time.Duration(intervalMin) * time.Minute)

			// Read enabled and countPerAccount under lock
			autoHMEStatus.mu.RLock()
			enabled := autoHMEStatus.Enabled
			countPerAcc := autoHMEStatus.CountPerAccount
			autoHMEStatus.mu.RUnlock()

			if enabled {
				addAutoHMELog("info", "开始执行定时自动创建 HME 任务...")
				AutoCreateHMEForAllAccounts(countPerAcc)
			} else {
				addAutoHMELog("warning", "自动创建任务已禁用，跳过本次执行")
			}
		}
	}()
	autoHMEStatus.mu.RLock()
	initInterval := autoHMEStatus.IntervalMinutes
	initCount := autoHMEStatus.CountPerAccount
	autoHMEStatus.mu.RUnlock()
	addAutoHMELog("info", fmt.Sprintf("定时自动创建 HME 任务已启动 (每 %d 分钟执行一次，每账户创建 %d 个)", initInterval, initCount))
}

// AutoCreateHMEForAllAccounts creates HME for all accounts with valid sessions
// This runs in a single goroutine, processing accounts sequentially (no concurrency)
func AutoCreateHMEForAllAccounts(countPerAccount int) {
	if store.DB == nil {
		addAutoHMELog("error", "数据库未连接，跳过执行")
		return
	}

	// Check and mark task as running (prevent overlapping runs)
	now := time.Now()
	autoHMEStatus.mu.Lock()
	if autoHMEStatus.Running {
		autoHMEStatus.mu.Unlock()
		addAutoHMELog("warning", "任务已在执行中，跳过本次运行")
		return
	}
	autoHMEStatus.Running = true
	autoHMEStatus.LastRunTime = &now
	autoHMEStatus.TotalCreated = 0
	autoHMEStatus.TotalFailed = 0
	autoHMEStatus.ProcessedAccounts = 0
	autoHMEStatus.CurrentAccount = ""
	autoHMEStatus.CurrentProgress = 0
	autoHMEStatus.mu.Unlock()

	defer func() {
		autoHMEStatus.mu.Lock()
		autoHMEStatus.Running = false
		autoHMEStatus.CurrentAccount = ""
		autoHMEStatus.CurrentProgress = 0
		autoHMEStatus.mu.Unlock()
	}()

	// Find all accounts with valid sessions (status=1 means logged in)
	var accounts []store.Account
	if err := store.DB.Where("session_token != '' AND session_token IS NOT NULL AND status = 1").Find(&accounts).Error; err != nil {
		addAutoHMELog("error", fmt.Sprintf("查询账户失败: %v", err))
		return
	}

	if len(accounts) == 0 {
		addAutoHMELog("warning", "没有找到有效会话的账户")
		return
	}

	autoHMEStatus.mu.Lock()
	autoHMEStatus.TotalAccounts = len(accounts)
	autoHMEStatus.mu.Unlock()

	addAutoHMELog("info", fmt.Sprintf("找到 %d 个账户，每个账户将创建 %d 个 HME...", len(accounts), countPerAccount))

	totalCreated := 0
	totalFailed := 0

	for idx, account := range accounts {
		autoHMEStatus.mu.Lock()
		autoHMEStatus.CurrentAccount = account.AppleID
		autoHMEStatus.CurrentProgress = 0
		autoHMEStatus.ProcessedAccounts = idx
		autoHMEStatus.mu.Unlock()

		addAutoHMELog("info", fmt.Sprintf("开始处理账户 [%d/%d]: %s", idx+1, len(accounts), account.AppleID))

		// Restore session
		auth := apple.RestoreAppleAuth(account.SessionToken, account.SessionSCNT, account.SessionID, account.SessionCookies)
		hme := apple.NewHMEClient(auth)
		hme.MarkRestoredSession() // Skip Bootstrap validation for restored sessions

		// First extend session to make sure it's valid
		ok, err := hme.ExtendSession()
		if !ok {
			addAutoHMELog("error", fmt.Sprintf("账户 %s 会话无效: %v，跳过", account.AppleID, err))
			continue
		}

		// Create HME one by one (synchronously, no concurrency)
		created := 0
		consecutiveFailed := 0
		failed := 0
		batchTS := time.Now().Format("0102-1504")
		for i := 0; i < countPerAccount; i++ {
			autoHMEStatus.mu.Lock()
			autoHMEStatus.CurrentProgress = i + 1
			autoHMEStatus.mu.Unlock()

			label := fmt.Sprintf("Auto-%s-%d", batchTS, i+1)
			email, err := hme.CreateEmail(label, "", "")
			if err != nil {
				addAutoHMELog("error", fmt.Sprintf("账户 %s: 创建第 %d 个 HME 失败: %v", account.AppleID, i+1, err))
				consecutiveFailed++
				failed++
				// If we get too many consecutive failures, stop for this account
				if consecutiveFailed >= 3 {
					addAutoHMELog("warning", fmt.Sprintf("账户 %s: 连续失败 %d 次，跳过剩余创建", account.AppleID, consecutiveFailed))
					break
				}
				time.Sleep(2 * time.Second) // Wait longer on failure
				continue
			}

			// Reset consecutive failure counter on success
			consecutiveFailed = 0

			// Save to database
			hmeRepo := store.NewHMERepo()
			hmeRepo.Create(&store.HMERecord{
				AccountID:      account.ID,
				HMEID:          email.ID,
				EmailAddress:   email.EmailAddress,
				Label:          email.Label,
				Note:           email.Note,
				ForwardToEmail: email.ForwardToEmail,
				Active:         email.Active,
			})

			created++

			// Delay between creations (1 second)
			if i < countPerAccount-1 {
				time.Sleep(1 * time.Second)
			}
		}

		// Update HME count for this account
		hmeRepo := store.NewHMERepo()
		count, _ := hmeRepo.Count(account.ID)
		store.NewAccountRepo().UpdateHMECount(account.ID, int(count))

		// Save session (may have new cookies)
		saveAppleSession(account.ID, auth)

		totalCreated += created
		totalFailed += failed

		autoHMEStatus.mu.Lock()
		autoHMEStatus.TotalCreated = totalCreated
		autoHMEStatus.TotalFailed = totalFailed
		autoHMEStatus.mu.Unlock()

		addAutoHMELog("success", fmt.Sprintf("账户 %s: 完成，创建成功=%d，失败=%d", account.AppleID, created, failed))

		// Small delay between accounts
		time.Sleep(2 * time.Second)
	}

	autoHMEStatus.mu.Lock()
	autoHMEStatus.ProcessedAccounts = len(accounts)
	autoHMEStatus.mu.Unlock()

	addAutoHMELog("success", fmt.Sprintf("定时任务完成: 共创建成功 %d 个，失败 %d 个", totalCreated, totalFailed))
}

// GetAutoHMEStatus returns the current auto HME task status
func (s *Server) GetAutoHMEStatus(c *gin.Context) {
	// Get eligible accounts OUTSIDE the lock (DB query can be slow)
	var eligibleAccounts []struct {
		AppleID      string `json:"appleId"`
		HMECount     int    `json:"hmeCount"`
		PhoneNumbers string `json:"phoneNumbers"`
	}
	if store.DB != nil {
		// Session must exist, status=1, and session_saved_at within maxSessionAge (7 days)
		minSessionTime := time.Now().Add(-maxSessionAge)
		store.DB.Model(&store.Account{}).
			Select("apple_id, hme_count, phone_numbers").
			Where("session_token != '' AND session_token IS NOT NULL AND status = 1 AND session_saved_at IS NOT NULL AND session_saved_at > ?", minSessionTime).
			Find(&eligibleAccounts)
	}

	// Read status fields under lock
	autoHMEStatus.mu.RLock()
	statusData := map[string]interface{}{
		"enabled":           autoHMEStatus.Enabled,
		"running":           autoHMEStatus.Running,
		"intervalMinutes":   autoHMEStatus.IntervalMinutes,
		"countPerAccount":   autoHMEStatus.CountPerAccount,
		"lastRunTime":       autoHMEStatus.LastRunTime,
		"nextRunTime":       autoHMEStatus.NextRunTime,
		"currentAccount":    autoHMEStatus.CurrentAccount,
		"currentProgress":   autoHMEStatus.CurrentProgress,
		"totalAccounts":     autoHMEStatus.TotalAccounts,
		"processedAccounts": autoHMEStatus.ProcessedAccounts,
		"totalCreated":      autoHMEStatus.TotalCreated,
		"totalFailed":       autoHMEStatus.TotalFailed,
		"eligibleAccounts":  eligibleAccounts,
	}
	autoHMEStatus.mu.RUnlock()

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    statusData,
	})
}

// GetAutoHMELogs returns the auto HME task logs
func (s *Server) GetAutoHMELogs(c *gin.Context) {
	autoHMEStatus.mu.RLock()
	defer autoHMEStatus.mu.RUnlock()
	
	// Return logs in reverse order (newest first)
	logs := make([]AutoHMELogEntry, len(autoHMEStatus.Logs))
	for i, entry := range autoHMEStatus.Logs {
		logs[len(autoHMEStatus.Logs)-1-i] = entry
	}
	
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    logs,
	})
}

// UpdateAutoHMESettings updates the auto HME task settings
func (s *Server) UpdateAutoHMESettings(c *gin.Context) {
	var req struct {
		Enabled         *bool `json:"enabled"`
		IntervalMinutes *int  `json:"intervalMinutes"`
		CountPerAccount *int  `json:"countPerAccount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	autoHMEStatus.mu.Lock()
	if req.Enabled != nil {
		autoHMEStatus.Enabled = *req.Enabled
	}
	if req.IntervalMinutes != nil && *req.IntervalMinutes >= 5 {
		autoHMEStatus.IntervalMinutes = *req.IntervalMinutes
	}
	if req.CountPerAccount != nil && *req.CountPerAccount >= 1 && *req.CountPerAccount <= 100 {
		autoHMEStatus.CountPerAccount = *req.CountPerAccount
	}
	// Capture values before unlock for logging
	enabled := autoHMEStatus.Enabled
	intervalMin := autoHMEStatus.IntervalMinutes
	countPerAcc := autoHMEStatus.CountPerAccount
	autoHMEStatus.mu.Unlock()

	addAutoHMELog("info", fmt.Sprintf("设置已更新: 启用=%v, 间隔=%d分钟, 每账户创建=%d个",
		enabled, intervalMin, countPerAcc))

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// TriggerAutoHME manually triggers the auto HME creation
func (s *Server) TriggerAutoHME(c *gin.Context) {
	autoHMEStatus.mu.Lock()
	if autoHMEStatus.Running {
		autoHMEStatus.mu.Unlock()
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "任务正在执行中"})
		return
	}
	// Mark as running under the same lock to prevent TOCTOU race
	autoHMEStatus.Running = true
	countPerAcc := autoHMEStatus.CountPerAccount
	autoHMEStatus.mu.Unlock()

	go func() {
		addAutoHMELog("info", "手动触发自动创建 HME 任务...")
		AutoCreateHMEForAllAccounts(countPerAcc)
	}()

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]string{"message": "任务已触发"}})
}

// hmeRecordToEmail converts a DB HMERecord to the Apple HMEEmail format expected by frontend
func hmeRecordToEmail(r store.HMERecord) apple.HMEEmail {
	return apple.HMEEmail{
		ID:             r.HMEID,
		EmailAddress:   r.EmailAddress,
		Label:          r.Label,
		Note:           r.Note,
		ForwardToEmail: r.ForwardToEmail,
		Active:         r.Active,
		CreateTime:     r.CreatedAt.UnixMilli(),
	}
}

// GetAccountHME gets HME list for an account
func (s *Server) GetAccountHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	// Try active session or restore from DB
	if ensureAppleSession(session, uint(id)) {
		emails, err := session.HME.ListEmails()
		if err != nil {
			errStr := err.Error()
			log.Printf("[GetAccountHME] Apple API error: %v", err)

			// If 401 error, session is invalid - clear it
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
				go clearInvalidSession(uint(id))
				session.HME = nil
				session.Auth = nil
				log.Printf("[GetAccountHME] Session expired for account %d, cleared", id)
			}
		} else {
			go syncHMEToDB(uint(id), emails)
			// Save session after successful HME operation (Bootstrap may have added new cookies)
			go saveAppleSession(uint(id), session.Auth)
			c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
			return
		}
	}

	// Fallback: fetch from database — convert to HMEEmail format for frontend consistency
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}
	records, err := store.NewHMERepo().FindByAccountID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}
	emails := make([]apple.HMEEmail, len(records))
	for i, r := range records {
		emails[i] = hmeRecordToEmail(r)
	}
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
}

// syncHMEToDB syncs Apple HME emails to the local database (upsert)
func syncHMEToDB(accountID uint, emails []apple.HMEEmail) {
	if store.DB == nil || len(emails) == 0 {
		return
	}
	hmeRepo := store.NewHMERepo()
	for _, e := range emails {
		existing, _ := hmeRepo.FindByHMEID(e.ID)
		if existing != nil {
			if err := store.DB.Model(existing).Updates(map[string]interface{}{
				"email_address":    e.EmailAddress,
				"label":            e.Label,
				"note":             e.Note,
				"forward_to_email": e.ForwardToEmail,
				"active":           e.Active,
			}).Error; err != nil {
				log.Printf("[syncHME] Failed to update HME %s: %v", e.ID, err)
			}
		} else {
			if err := hmeRepo.Create(&store.HMERecord{
				AccountID:      accountID,
				HMEID:          e.ID,
				EmailAddress:   e.EmailAddress,
				Label:          e.Label,
				Note:           e.Note,
				ForwardToEmail: e.ForwardToEmail,
				Active:         e.Active,
			}); err != nil {
				log.Printf("[syncHME] Failed to create HME %s: %v", e.ID, err)
			}
		}
	}
	// Update count
	count, _ := hmeRepo.Count(accountID)
	store.NewAccountRepo().UpdateHMECount(accountID, int(count))
}

// CreateAccountHME creates HME for an account
func (s *Server) CreateAccountHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req CreateHMERequest
	c.ShouldBindJSON(&req)

	hme, err := session.HME.CreateEmail(req.Label, req.Note, req.ForwardToEmail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Save to database
	hmeRepo := store.NewHMERepo()
	hmeRepo.Create(&store.HMERecord{
		AccountID:      uint(id),
		HMEID:          hme.ID,
		EmailAddress:   hme.EmailAddress,
		Label:          hme.Label,
		Note:           hme.Note,
		ForwardToEmail: hme.ForwardToEmail,
		Active:         hme.Active,
	})

	// Update count
	count, _ := hmeRepo.Count(uint(id))
	store.NewAccountRepo().UpdateHMECount(uint(id), int(count))

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: hme})
}

// DeleteAccountHME deletes an HME for an account
func (s *Server) DeleteAccountHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	accountID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	hmeId := c.Param("hmeId")
	if accountID == 0 || hmeId == "" {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的参数"})
		return
	}

	localOnly := false
	// Try to delete from Apple if session is active (or can be restored)
	if ensureAppleSession(session, uint(accountID)) {
		if err := session.HME.DeleteEmail(hmeId); err != nil {
			c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "Apple API 删除失败: " + err.Error()})
			return
		}
	} else {
		localOnly = true
	}

	// Delete from database
	hmeRepo := store.NewHMERepo()
	hmeRepo.DeleteByHMEID(hmeId)

	// Update count
	count, _ := hmeRepo.Count(uint(accountID))
	store.NewAccountRepo().UpdateHMECount(uint(accountID), int(count))

	if localOnly {
		c.JSON(http.StatusOK, APIResponse{Success: true, Data: map[string]string{
			"warning": "仅从本地数据库删除，Apple 端可能仍存在（请先登录账户）",
		}})
	} else {
		c.JSON(http.StatusOK, APIResponse{Success: true})
	}
}

// AdminStats returns aggregate dashboard stats
func (s *Server) AdminStats(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	var totalAccounts, activeAccounts, errorAccounts, totalHME int64
	store.DB.Model(&store.Account{}).Count(&totalAccounts)
	store.DB.Model(&store.Account{}).Where("status = 1").Count(&activeAccounts)
	store.DB.Model(&store.Account{}).Where("status != 1").Count(&errorAccounts)
	store.DB.Model(&store.HMERecord{}).Count(&totalHME)

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]int64{
			"totalAccounts":  totalAccounts,
			"activeAccounts": activeAccounts,
			"errorAccounts":  errorAccounts,
			"totalHME":       totalHME,
		},
	})
}

// AdminListAllHME returns global HME list with account info
func (s *Server) AdminListAllHME(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	pageSize = clampPageSize(pageSize)
	search := c.Query("search")

	type HMEWithAccount struct {
		store.HMERecord
		AppleID string `json:"appleId"`
	}

	var records []HMEWithAccount
	var total int64

	// Count query (without Select to avoid GORM Count+Select conflict)
	countQuery := store.DB.Table("hme_records").
		Joins("LEFT JOIN accounts ON accounts.id = hme_records.account_id").
		Where("hme_records.deleted_at IS NULL")
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		countQuery = countQuery.Where(
			"hme_records.email_address LIKE ? ESCAPE '\\' OR hme_records.label LIKE ? ESCAPE '\\' OR accounts.apple_id LIKE ? ESCAPE '\\'",
			like, like, like,
		)
	}
	countQuery.Count(&total)

	// Data query
	dataQuery := store.DB.Table("hme_records").
		Select("hme_records.*, accounts.apple_id").
		Joins("LEFT JOIN accounts ON accounts.id = hme_records.account_id").
		Where("hme_records.deleted_at IS NULL")
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		dataQuery = dataQuery.Where(
			"hme_records.email_address LIKE ? ESCAPE '\\' OR hme_records.label LIKE ? ESCAPE '\\' OR accounts.apple_id LIKE ? ESCAPE '\\'",
			like, like, like,
		)
	}

	offset := (page - 1) * pageSize
	err := dataQuery.Offset(offset).Limit(pageSize).Order("hme_records.created_at DESC").Find(&records).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"list":     records,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// AdminChangePassword changes admin password
func (s *Server) AdminChangePassword(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "密码至少6位"})
		return
	}

	adminRepo := store.NewAdminRepo()
	admin, err := adminRepo.FindByID(session.AdminID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "用户不存在"})
		return
	}

	if !admin.CheckPassword(req.OldPassword) {
		c.JSON(http.StatusOK, APIResponse{Success: false, Error: "原密码错误"})
		return
	}

	if err := admin.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "密码加密失败"})
		return
	}

	if err := store.DB.Model(admin).Update("password", admin.Password).Error; err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "保存失败"})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// GetAccountForwardEmails returns available forward-to emails for an account
func (s *Server) GetAccountForwardEmails(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	emails, err := session.HME.GetForwardEmails()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
}

// BatchCreateAccountHME batch creates HME for an account
func (s *Server) BatchCreateAccountHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.LabelPrefix == "" {
		req.LabelPrefix = "Auto"
	}
	if req.DelayMs == 0 {
		req.DelayMs = 1000
	}

	results, errors := session.HME.BatchCreateEmails(req.Count, req.LabelPrefix, req.DelayMs, req.ForwardToEmail)

	// Save to database
	if len(results) > 0 {
		hmeRepo := store.NewHMERepo()
		records := make([]store.HMERecord, len(results))
		for i, hme := range results {
			records[i] = store.HMERecord{
				AccountID:      uint(id),
				HMEID:          hme.ID,
				EmailAddress:   hme.EmailAddress,
				Label:          hme.Label,
				Note:           hme.Note,
				ForwardToEmail: hme.ForwardToEmail,
				Active:         hme.Active,
			}
		}
		hmeRepo.BatchCreate(records)

		// Update count
		count, _ := hmeRepo.Count(uint(id))
		store.NewAccountRepo().UpdateHMECount(uint(id), int(count))
	}

	errorStrings := make([]string, len(errors))
	for i, err := range errors {
		errorStrings[i] = err.Error()
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

// SendAlternateEmailVerification sends a verification code to add an alternate email
func (s *Server) SendAlternateEmailVerification(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请输入有效的邮箱地址"})
		return
	}

	result, err := session.HME.SendAlternateEmailVerification(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"verificationId": result.VerificationID,
			"address":        result.Address,
			"length":         result.Length,
		},
	})
}

// VerifyAlternateEmail verifies the code and adds the alternate email
func (s *Server) VerifyAlternateEmail(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req struct {
		Email          string `json:"email" binding:"required"`
		VerificationID string `json:"verificationId" binding:"required"`
		Code           string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "参数不完整"})
		return
	}

	result, err := session.HME.VerifyAlternateEmail(req.Email, req.VerificationID, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Update account's alternate emails in database
	go func() {
		if session.HME != nil {
			fetchAndSaveAccountInfo(uint(id), session.HME)
		}
	}()

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"address": result.Address,
			"vetted":  result.Vetted,
		},
	})
}

// RefreshAccountInfo refreshes account profile info from Apple
func (s *Server) RefreshAccountInfo(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	// Fetch and save account info synchronously
	fetchAndSaveAccountInfo(uint(id), session.HME)

	// Return updated account
	account, err := store.NewAccountRepo().FindByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "获取账户失败"})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: account})
}

// RemoveAlternateEmail removes an alternate email from the account
func (s *Server) RemoveAlternateEmail(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请输入邮箱地址"})
		return
	}

	if err := session.HME.RemoveAlternateEmail(req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Update account's alternate emails in database
	go func() {
		if session.HME != nil {
			fetchAndSaveAccountInfo(uint(id), session.HME)
		}
	}()

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// GetFamilyMembers returns family sharing members for an account
func (s *Server) GetFamilyMembers(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	familyResp, err := session.HME.GetFamilyMembers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: familyResp})
}

// GetForwardEmailOptions returns available forward-to email options
func (s *Server) GetForwardEmailOptions(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	result, err := session.HME.GetForwardEmailOptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: result})
}

// SetForwardEmail sets the forward-to email address
func (s *Server) SetForwardEmail(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if !ensureAppleSession(session, uint(id)) {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请先登录此Apple账户"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "请选择转发邮箱"})
		return
	}

	if err := session.HME.SetForwardEmail(req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// GetSystemSettings returns system settings
func (s *Server) GetSystemSettings(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	proxyURL, _ := store.GetSetting("proxy_url")

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]string{
			"proxyUrl": proxyURL,
		},
	})
}

// UpdateSystemSettings updates system settings
func (s *Server) UpdateSystemSettings(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	var req struct {
		ProxyURL string `json:"proxyUrl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if err := store.SetSetting("proxy_url", req.ProxyURL); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "保存失败"})
		return
	}

	log.Printf("[Settings] Proxy URL updated: %s", req.ProxyURL)
	c.JSON(http.StatusOK, APIResponse{Success: true})
}

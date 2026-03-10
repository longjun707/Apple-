package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
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

// escapeLike escapes SQL LIKE wildcards.
func escapeLike(s string) string {
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

// ensureAppleSession checks if Apple session is active for the account, tries to restore from DB if not
func ensureAppleSession(session *SessionState, accountID uint) bool {
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
	auth := apple.RestoreAppleAuth(account.SessionToken, account.SessionSCNT, account.SessionID, account.SessionCookies)
	hme := apple.NewHMEClient(auth)
	session.Auth = auth
	session.HME = hme
	session.AccountID = accountID
	session.AppleID = account.AppleID
	log.Printf("[Session] Restored Apple session for account %d from DB", accountID)
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
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"id":       session.AdminID,
			"username": session.AdminName,
		},
	})
}

// ListAccounts returns all Apple accounts
func (s *Server) ListAccounts(c *gin.Context) {
	if store.DB == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "数据库未连接"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	pageSize = clampPageSize(pageSize)

	accounts, total, err := store.NewAccountRepo().List(page, pageSize)
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

	account := &store.Account{
		AppleID:  req.AppleID,
		Password: req.Password,
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

	updates := map[string]interface{}{
		"apple_id": req.AppleID,
		"remark":   req.Remark,
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}

	if err := store.DB.Model(&store.Account{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true})
}

// DeleteAccount deletes an Apple account
func (s *Server) DeleteAccount(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "无效的ID"})
		return
	}

	if err := store.DB.Delete(&store.Account{}, id).Error; err != nil {
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

	// Create Apple auth client
	auth := apple.NewAppleAuth()
	result, err := auth.Login(account.AppleID, account.Password)

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

	// Persist session to database
	go saveAppleSession(uint(id), auth)

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
	if session.Auth == nil {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "请先登录Apple账户"})
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

	// Update account status
	store.DB.Model(&store.Account{}).Where("id = ?", session.AccountID).Updates(map[string]interface{}{
		"status":     1,
		"last_error": "",
	})

	// Persist full Apple session (tokens + cookies) to database
	go saveAppleSession(session.AccountID, session.Auth)

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

// GetAccountHME gets HME list for an account
func (s *Server) GetAccountHME(c *gin.Context) {
	session := c.MustGet("session").(*SessionState)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	// Try active session or restore from DB
	if ensureAppleSession(session, uint(id)) {
		emails, err := session.HME.ListEmails()
		if err != nil {
			log.Printf("[GetAccountHME] Apple API error (falling back to DB): %v", err)
		} else {
			go syncHMEToDB(uint(id), emails)
			c.JSON(http.StatusOK, APIResponse{Success: true, Data: emails})
			return
		}
	}

	// Fallback: fetch from database
	records, err := store.NewHMERepo().FindByAccountID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: records})
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
			// Update if changed
			store.DB.Model(existing).Updates(map[string]interface{}{
				"email_address":    e.EmailAddress,
				"label":            e.Label,
				"note":             e.Note,
				"forward_to_email": e.ForwardToEmail,
				"active":           e.Active,
			})
		} else {
			hmeRepo.Create(&store.HMERecord{
				AccountID:      accountID,
				HMEID:          e.ID,
				EmailAddress:   e.EmailAddress,
				Label:          e.Label,
				Note:           e.Note,
				ForwardToEmail: e.ForwardToEmail,
				Active:         e.Active,
			})
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

	// Try to delete from Apple if session is active (or can be restored)
	if ensureAppleSession(session, uint(accountID)) {
		if err := session.HME.DeleteEmail(hmeId); err != nil {
			c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "Apple API 删除失败: " + err.Error()})
			return
		}
	}

	// Delete from database
	hmeRepo := store.NewHMERepo()
	hmeRepo.DeleteByHMEID(hmeId)

	// Update count
	count, _ := hmeRepo.Count(uint(accountID))
	store.NewAccountRepo().UpdateHMECount(uint(accountID), int(count))

	c.JSON(http.StatusOK, APIResponse{Success: true})
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
		countQuery = countQuery.Where("hme_records.email_address LIKE ? OR hme_records.label LIKE ? OR accounts.apple_id LIKE ? ESCAPE '\\'", like, like, like)
	}
	countQuery.Count(&total)

	// Data query
	dataQuery := store.DB.Table("hme_records").
		Select("hme_records.*, accounts.apple_id").
		Joins("LEFT JOIN accounts ON accounts.id = hme_records.account_id").
		Where("hme_records.deleted_at IS NULL")
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		dataQuery = dataQuery.Where("hme_records.email_address LIKE ? OR hme_records.label LIKE ? OR accounts.apple_id LIKE ? ESCAPE '\\'", like, like, like)
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
	if session.AdminID == 0 {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "未登录"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

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

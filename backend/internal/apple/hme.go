package apple

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// HMEClient handles Hide My Email operations
type HMEClient struct {
	auth            *AppleAuth
	bootstrapped    bool
	restoredSession bool // true if session was restored from DB (skip strict validation)
}

// NewHMEClient creates a new HME client from authenticated session
func NewHMEClient(auth *AppleAuth) *HMEClient {
	return &HMEClient{
		auth: auth,
	}
}

// MarkRestoredSession marks this client as having a restored session (skip strict Bootstrap validation)
func (c *HMEClient) MarkRestoredSession() {
	c.restoredSession = true
}

// ExtendSession calls Apple's session extend endpoint to refresh the session
// Returns true if session is still valid
func (c *HMEClient) ExtendSession() (bool, error) {
	// Ensure cookies are set
	c.ensureMyacinfo()

	// Call session extend endpoint (actual Apple endpoint: /session/extend)
	extendURL := "https://appleid.apple.com/session/extend"
	req, _ := http.NewRequest("GET", extendURL, nil)

	// Use same headers as browser (observed from Chrome DevTools)
	req.Header.Set("User-Agent", ChromeUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://account.apple.com")
	req.Header.Set("Referer", "https://account.apple.com/")
	req.Header.Set("X-Apple-Api-Key", HMEWidgetKey)
	req.Header.Set("X-Apple-I-Request-Context", "ca")
	req.Header.Set("X-Apple-I-Timezone", "Asia/Shanghai")
	req.Header.Set("X-Apple-I-FD-Client-Info", `{"U":"`+ChromeUA+`","L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":""}`)
	// Security headers like browser
	req.Header.Set("Sec-Ch-Ua", `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	// Include scnt header if available
	c.auth.session.mu.RLock()
	if c.auth.session.SCNT != "" {
		req.Header.Set("scnt", c.auth.session.SCNT)
	}
	c.auth.session.mu.RUnlock()

	resp, err := c.auth.session.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("session extend request failed: %w", err)
	}
	defer resp.Body.Close()

	// Update SCNT from response
	if scnt := resp.Header.Get("scnt"); scnt != "" {
		c.auth.session.mu.Lock()
		c.auth.session.SCNT = scnt
		c.auth.session.mu.Unlock()
	}

	log.Printf("[HME] Session extend: status=%d", resp.StatusCode)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return true, nil
	} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return false, fmt.Errorf("session expired (status %d)", resp.StatusCode)
	}

	return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// GetAuth returns the underlying AppleAuth for session export
func (c *HMEClient) GetAuth() *AppleAuth {
	return c.auth
}

// hmeHeaders returns headers for HME API requests (always includes API key like Python)
func (c *HMEClient) hmeHeaders() map[string]string {
	headers := map[string]string{
		"Accept":                    "application/json",
		"Content-Type":              "application/json",
		"Origin":                    "https://account.apple.com",
		"Referer":                   "https://account.apple.com/",
		"User-Agent":                ChromeUA,
		"X-Apple-I-Request-Context": "ca",
		"X-Apple-I-Timezone":        "Asia/Shanghai",
		"X-Apple-I-FD-Client-Info":  `{"U":"` + ChromeUA + `","L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":""}`,
		// Security headers like browser
		"Sec-Ch-Ua":          `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-site",
	}

	c.auth.session.mu.RLock()
	if c.auth.session.SCNT != "" {
		headers["scnt"] = c.auth.session.SCNT
	}
	if c.auth.session.SessionID != "" {
		headers["X-Apple-ID-Session-Id"] = c.auth.session.SessionID
	}
	c.auth.session.mu.RUnlock()

	return headers
}

// doRequest performs an HME API request
func (c *HMEClient) doRequest(method, urlPath string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, urlPath, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range c.hmeHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.auth.session.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// Update SCNT from response
	if scnt := resp.Header.Get("scnt"); scnt != "" {
		c.auth.session.mu.Lock()
		c.auth.session.SCNT = scnt
		c.auth.session.mu.Unlock()
	}

	return resp, nil
}

// Bootstrap initializes the HME session
func (c *HMEClient) Bootstrap() error {
	if c.bootstrapped {
		return nil
	}

	// Ensure myacinfo cookie exists
	c.ensureMyacinfo()

	// CRITICAL: Set idclient=web cookie on appleid.apple.com (browser does this)
	appleidURL, _ := url.Parse("https://appleid.apple.com")
	c.auth.session.Client.Jar.SetCookies(appleidURL, []*http.Cookie{{
		Name:  "idclient",
		Value: "web",
		Path:  "/",
	}})

	// For restored sessions, skip the full Bootstrap flow - just set cookies and try API directly
	if c.restoredSession {
		log.Printf("[HME] Restored session: skipping full Bootstrap, just setting cookies")
		c.bootstrapped = true
		return nil
	}

	// Step 1: Bootstrap portal
	log.Printf("[HME] Bootstrap: calling portal...")
	resp, err := c.doRequest("GET", BootstrapURL, nil)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("[HME] Bootstrap portal: status=%d", resp.StatusCode)
	if resp.StatusCode != 200 {
		return fmt.Errorf("bootstrap failed: HTTP %d - %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	// Capture aidsp cookie from bootstrap response
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "aidsp" {
			c.auth.session.Client.Jar.SetCookies(appleidURL, []*http.Cookie{{
				Name:  "aidsp",
				Value: cookie.Value,
				Path:  "/",
			}})
		}
	}

	// Step 2: Token exchange (non-fatal - HME API may work without it)
	log.Printf("[HME] Bootstrap: calling token exchange...")
	tokenResp, err := c.doRequest("GET", TokenURL, nil)
	if err != nil {
		log.Printf("[HME] token exchange error (non-fatal): %v", err)
	} else {
		tokenBody, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()
		log.Printf("[HME] Token exchange: status=%d, body_len=%d", tokenResp.StatusCode, len(tokenBody))
	}

	c.bootstrapped = true
	log.Printf("[HME] bootstrap complete")
	return nil
}

// exchangeAccountSession exchanges idmsa session for account.apple.com session
func (c *HMEClient) exchangeAccountSession() error {
	authURL := AuthBase + "/authorize/signin" +
		"?client_id=" + HMEWidgetKey +
		"&redirect_uri=https%3A%2F%2Faccount.apple.com" +
		"&response_type=code" +
		"&response_mode=web_message"

	// Use account.apple.com specific headers, not iCloud headers
	c.auth.session.mu.RLock()
	headers := map[string]string{
		"Accept":                            "application/json",
		"Content-Type":                      "application/json",
		"User-Agent":                        ChromeUA,
		"X-Apple-OAuth-Client-Id":           HMEWidgetKey,
		"X-Apple-OAuth-Client-Type":         "firstPartyAuth",
		"X-Apple-OAuth-Redirect-URI":        "https://account.apple.com",
		"X-Apple-OAuth-Require-Grant-Code":  "true",
		"X-Apple-OAuth-Response-Mode":       "web_message",
		"X-Apple-OAuth-Response-Type":       "code",
		"X-Apple-Widget-Key":                HMEWidgetKey,
		"X-Requested-With":                  "XMLHttpRequest",
		"Origin":                            "https://account.apple.com",
		"Referer":                           "https://account.apple.com/",
	}
	if c.auth.session.SCNT != "" {
		headers["scnt"] = c.auth.session.SCNT
	}
	if c.auth.session.SessionID != "" {
		headers["X-Apple-ID-Session-Id"] = c.auth.session.SessionID
	}
	c.auth.session.mu.RUnlock()

	req, _ := http.NewRequest("GET", authURL, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.auth.session.Client.Do(req)
	if err != nil {
		return fmt.Errorf("account session exchange failed: %w", err)
	}
	defer resp.Body.Close()
	c.auth.captureSessionHeaders(resp)
	exBody, _ := io.ReadAll(resp.Body)
	srpLog.Printf("[HME] account session exchange: status=%d, body=%s", resp.StatusCode, string(exBody[:min(len(exBody), 500)]))

	// If authorize/signin failed, try visiting account.apple.com directly (like Python fallback)
	if resp.StatusCode != 200 {
		srpLog.Printf("[HME] trying account.apple.com fallback...")
		fallbackReq, _ := http.NewRequest("GET", "https://account.apple.com/", nil)
		fallbackReq.Header.Set("User-Agent", ChromeUA)
		fallbackReq.Header.Set("Accept", "text/html,application/xhtml+xml")
		fresp, ferr := c.auth.session.Client.Do(fallbackReq)
		if ferr == nil {
			io.ReadAll(fresp.Body)
			fresp.Body.Close()
			srpLog.Printf("[HME] account.apple.com fallback: status=%d", fresp.StatusCode)
		}
	}
	return nil
}

// logCookies logs relevant cookies for debugging
func (c *HMEClient) logCookies() {
	domains := []string{"https://apple.com", "https://idmsa.apple.com", "https://appleid.apple.com", "https://account.apple.com"}
	for _, d := range domains {
		u, _ := url.Parse(d)
		cookies := c.auth.session.Client.Jar.Cookies(u)
		names := make([]string, 0, len(cookies))
		for _, ck := range cookies {
			names = append(names, ck.Name)
		}
		if len(names) > 0 {
			srpLog.Printf("[HME] cookies for %s: %v", d, names)
		}
	}
	c.auth.session.mu.RLock()
	srpLog.Printf("[HME] SessionToken=%v, SCNT=%v", c.auth.session.SessionToken != "", c.auth.session.SCNT != "")
	c.auth.session.mu.RUnlock()
}

// ensureMyacinfo sets myacinfo cookie if we have session token
func (c *HMEClient) ensureMyacinfo() {
	c.auth.session.mu.Lock()
	defer c.auth.session.mu.Unlock()

	if c.auth.session.SessionToken == "" {
		return
	}

	// Check if myacinfo already exists
	u, _ := url.Parse("https://apple.com")
	cookies := c.auth.session.Client.Jar.Cookies(u)
	for _, cookie := range cookies {
		if cookie.Name == "myacinfo" {
			return
		}
	}

	// Set myacinfo cookie
	cookie := &http.Cookie{
		Name:   "myacinfo",
		Value:  c.auth.session.SessionToken,
		Domain: ".apple.com",
		Path:   "/",
	}
	c.auth.session.Client.Jar.SetCookies(u, []*http.Cookie{cookie})
}

// ListEmails returns all HME addresses
func (c *HMEClient) ListEmails() ([]HMEEmail, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	resp, err := c.doRequest("GET", AccountBase+"/email/private", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list HME failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result HMEListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.PrivateEmailList, nil
}

// GenerateEmail generates a new random HME address (not activated yet)
func (c *HMEClient) GenerateEmail() (string, error) {
	if err := c.Bootstrap(); err != nil {
		return "", err
	}

	resp, err := c.doRequest("POST", AccountBase+"/email/private/add", map[string]interface{}{})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("generate HME failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.EmailAddress, nil
}

// CompleteEmail activates a generated HME address
func (c *HMEClient) CompleteEmail(email, label, note, forwardToEmail string) (*HMEEmail, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	req := HMECreateRequest{
		EmailAddress:   email,
		Label:          label,
		Note:           note,
		ForwardToEmail: forwardToEmail,
	}

	resp, err := c.doRequest("PUT", AccountBase+"/email/private/add/complete", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("complete HME failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result HMEEmail
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateEmail creates and activates a new HME address (full flow)
func (c *HMEClient) CreateEmail(label, note, forwardToEmail string) (*HMEEmail, error) {
	// Step 1: Generate address
	email, err := c.GenerateEmail()
	if err != nil {
		return nil, err
	}

	// Auto-generate label if empty
	if label == "" {
		label = fmt.Sprintf("Auto-%d", time.Now().Unix()%100000)
	}
	if note == "" {
		note = time.Now().Format("Auto 2006-01-02 15:04")
	}

	// Step 2: Complete creation
	return c.CompleteEmail(email, label, note, forwardToEmail)
}

// DeleteEmail deletes an HME address
func (c *HMEClient) DeleteEmail(id string) error {
	if err := c.Bootstrap(); err != nil {
		return err
	}

	req := map[string]string{"id": id}
	resp, err := c.doRequest("POST", AccountBase+"/email/private/delete", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete HME failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetForwardEmails returns available forward-to email addresses
func (c *HMEClient) GetForwardEmails() ([]string, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	// Try the email/private endpoint first (same as list emails)
	resp, err := c.doRequest("GET", AccountBase+"/email/private", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[HME] GetForwardEmails failed: HTTP %d - %s", resp.StatusCode, string(body[:min(len(body), 200)]))
		return nil, fmt.Errorf("get forward emails failed: HTTP %d", resp.StatusCode)
	}

	// Parse the response to get forwardToEmail from any existing HME
	var listResult HMEListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		return nil, err
	}

	// Extract unique forward emails from existing HME addresses
	emailSet := make(map[string]bool)
	for _, hme := range listResult.PrivateEmailList {
		if hme.ForwardToEmail != "" {
			emailSet[hme.ForwardToEmail] = true
		}
	}

	// Convert to slice
	emails := make([]string, 0, len(emailSet))
	for email := range emailSet {
		emails = append(emails, email)
	}

	return emails, nil
}

// GetAccountInfo returns account information
func (c *HMEClient) GetAccountInfo() (*AccountInfo, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	resp, err := c.doRequest("GET", AccountBase, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get account info failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// BatchCreateEmails creates multiple HME addresses
func (c *HMEClient) BatchCreateEmails(count int, labelPrefix string, delayMs int, forwardToEmail string) ([]HMEEmail, []error) {
	results := make([]HMEEmail, 0, count)
	errors := make([]error, 0)

	for i := 0; i < count; i++ {
		label := fmt.Sprintf("%s-%d", labelPrefix, i+1)
		note := time.Now().Format("Auto 2006-01-02 15:04")

		hme, err := c.CreateEmail(label, note, forwardToEmail)
		if err != nil {
			errors = append(errors, fmt.Errorf("email %d: %w", i+1, err))
		} else {
			results = append(results, *hme)
		}

		// Delay between creations
		if i < count-1 && delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	return results, errors
}

// GetAccountProfile returns aggregated account profile info
func (c *HMEClient) GetAccountProfile() (*AccountProfileInfo, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	profile := &AccountProfileInfo{}

	// 1. Get /account/manage for basic info
	manageResp, err := c.doRequest("GET", AccountBase, nil)
	if err == nil {
		if manageResp.StatusCode == 200 {
			// Read body for debugging
			body, _ := io.ReadAll(manageResp.Body)
			log.Printf("[Profile] /account/manage response (first 1000 chars): %s", string(body[:min(len(body), 1000)]))

			var manageData AccountManageResponse
			if json.Unmarshal(body, &manageData) == nil {
				profile.Birthday = manageData.LocalizedBirthday
				profile.Country = manageData.PageFeatures.DefaultCountry
				log.Printf("[Profile] Found %d alternate email addresses", len(manageData.AlternateEmailAddresses))
				for _, email := range manageData.AlternateEmailAddresses {
					log.Printf("[Profile] Alternate email: %s, type=%s, vetted=%v", email.Address, email.Type, email.Vetted)
					if email.Vetted {
						profile.AlternateEmails = append(profile.AlternateEmails, email.Address)
					}
				}
			}
		} else {
			body, _ := io.ReadAll(manageResp.Body)
			log.Printf("[Profile] /account/manage failed: HTTP %d - %s", manageResp.StatusCode, string(body[:min(len(body), 200)]))
		}
		manageResp.Body.Close()
	}

	// 2. Get devices count
	devResp, err := c.doRequest("GET", AccountBase+"/security/devices", nil)
	if err == nil {
		if devResp.StatusCode == 200 {
			var devicesData DevicesResponse
			if json.NewDecoder(devResp.Body).Decode(&devicesData) == nil {
				profile.TrustedDeviceCount = len(devicesData.Devices)
			}
		}
		devResp.Body.Close()
	}

	return profile, nil
}

// GetFamilyMembers returns family sharing info
func (c *HMEClient) GetFamilyMembers() (*FamilyResponse, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	// First, call TokenURL to refresh caw/caw-at cookies (they expire in 5-15 minutes)
	// This is necessary because Family API requires fresh caw/caw-at cookies
	log.Printf("[Family] Refreshing caw/caw-at cookies via TokenURL...")
	tokenResp, err := c.doRequest("GET", TokenURL, nil)
	if err != nil {
		log.Printf("[Family] Token refresh failed: %v", err)
	} else {
		tokenResp.Body.Close()
		log.Printf("[Family] Token refresh: status=%d", tokenResp.StatusCode)
	}

	// Copy all relevant cookies from apple.com domains to familyws.icloud.apple.com
	familyURL, _ := url.Parse("https://familyws.icloud.apple.com")
	appleidURL, _ := url.Parse("https://appleid.apple.com")
	appleURL, _ := url.Parse("https://apple.com")

	// Get cookies from apple.com and appleid.apple.com
	var cookiesToCopy []*http.Cookie
	for _, cookies := range [][]*http.Cookie{
		c.auth.session.Client.Jar.Cookies(appleidURL),
		c.auth.session.Client.Jar.Cookies(appleURL),
	} {
		for _, cookie := range cookies {
			// Copy important cookies: myacinfo, caw, caw-at, acn01, dslang, site
			if cookie.Name == "myacinfo" || cookie.Name == "caw" || cookie.Name == "caw-at" ||
				cookie.Name == "acn01" || cookie.Name == "dslang" || cookie.Name == "site" {
				cookiesToCopy = append(cookiesToCopy, &http.Cookie{
					Name:  cookie.Name,
					Value: cookie.Value,
					Path:  "/",
				})
			}
		}
	}

	// Also ensure SessionToken is included as myacinfo
	c.auth.session.mu.RLock()
	token := c.auth.session.SessionToken
	c.auth.session.mu.RUnlock()
	if token != "" {
		cookiesToCopy = append(cookiesToCopy, &http.Cookie{
			Name:  "myacinfo",
			Value: token,
			Path:  "/",
		})
	}

	c.auth.session.Client.Jar.SetCookies(familyURL, cookiesToCopy)

	// Build request
	req, err := http.NewRequest("GET", "https://familyws.icloud.apple.com/api/family-members", nil)
	if err != nil {
		return nil, err
	}

	// Set headers similar to browser
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", ChromeUA)
	req.Header.Set("Referer", "https://familyws.icloud.apple.com/members?wid=d&env=idms_prod_account&theme=light&locale=zh_CN")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := c.auth.session.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get family members failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[Family] get family members failed: HTTP %d - %s", resp.StatusCode, string(body[:min(len(body), 200)]))
		return nil, fmt.Errorf("get family members failed: HTTP %d", resp.StatusCode)
	}

	var result FamilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode family response: %w", err)
	}

	log.Printf("[Family] got %d family members, isOrganizer=%v", len(result.FamilyMembers), result.Family != nil && result.Family.OrganizerDsid == result.CurrentDsid)
	return &result, nil
}

// alternateEmailHeaders returns headers for alternate email API requests
func (c *HMEClient) alternateEmailHeaders() map[string]string {
	headers := map[string]string{
		"Accept":                    "application/json, text/plain, */*",
		"Content-Type":              "application/json",
		"Origin":                    "https://account.apple.com",
		"Referer":                   "https://account.apple.com/",
		"User-Agent":                ChromeUA,
		"X-Apple-Api-Key":           HMEWidgetKey,
		"X-Apple-I-Request-Context": "ca",
		"X-Apple-I-Timezone":        "Asia/Shanghai",
		"X-Apple-I-FD-Client-Info":  `{"U":"` + ChromeUA + `","L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":""}`,
		// Security headers
		"Sec-Ch-Ua":          `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-site",
	}

	c.auth.session.mu.RLock()
	if c.auth.session.SCNT != "" {
		headers["scnt"] = c.auth.session.SCNT
	}
	c.auth.session.mu.RUnlock()

	return headers
}

// doAlternateEmailRequest performs a request to the alternate email API
func (c *HMEClient) doAlternateEmailRequest(method, urlPath string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, urlPath, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range c.alternateEmailHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.auth.session.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// Update SCNT from response
	if scnt := resp.Header.Get("scnt"); scnt != "" {
		c.auth.session.mu.Lock()
		c.auth.session.SCNT = scnt
		c.auth.session.mu.Unlock()
	}

	return resp, nil
}

// SendAlternateEmailVerification sends a verification code to the specified email address
func (c *HMEClient) SendAlternateEmailVerification(email string) (*AlternateEmailAddResponse, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	reqBody := map[string]string{"address": email}
	resp, err := c.doAlternateEmailRequest("POST", AccountBase+"/email/alternate/add/verification", reqBody)
	if err != nil {
		return nil, fmt.Errorf("send verification failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[AlternateEmail] Send verification to %s: status=%d, body=%s", email, resp.StatusCode, string(body[:min(len(body), 500)]))

	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("send verification failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result AlternateEmailAddResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// VerifyAlternateEmail verifies the code sent to the alternate email
func (c *HMEClient) VerifyAlternateEmail(email, verificationID, code string) (*AlternateEmailVerifyResponse, error) {
	if err := c.Bootstrap(); err != nil {
		return nil, err
	}

	reqBody := map[string]interface{}{
		"address": email,
		"verificationInfo": map[string]string{
			"id":     verificationID,
			"answer": code,
		},
	}

	resp, err := c.doAlternateEmailRequest("PUT", AccountBase+"/email/alternate/verification", reqBody)
	if err != nil {
		return nil, fmt.Errorf("verify email failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[AlternateEmail] Verify %s: status=%d, body=%s", email, resp.StatusCode, string(body[:min(len(body), 500)]))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("verify email failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result AlternateEmailVerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// RemoveAlternateEmail removes an alternate email address
func (c *HMEClient) RemoveAlternateEmail(email string) error {
	if err := c.Bootstrap(); err != nil {
		return err
	}

	reqBody := map[string]string{"address": email}
	resp, err := c.doAlternateEmailRequest("DELETE", AccountBase+"/email/alternate", reqBody)
	if err != nil {
		return fmt.Errorf("remove email failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[AlternateEmail] Remove %s: status=%d", email, resp.StatusCode)

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("remove email failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

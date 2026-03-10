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
	auth         *AppleAuth
	bootstrapped bool
}

// NewHMEClient creates a new HME client from authenticated session
func NewHMEClient(auth *AppleAuth) *HMEClient {
	return &HMEClient{
		auth: auth,
	}
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
		log.Printf("[HME] Using scnt (len=%d)", len(c.auth.session.SCNT))
	} else {
		log.Printf("[HME] WARNING: No scnt available!")
	}
	if c.auth.session.SessionID != "" {
		headers["X-Apple-ID-Session-Id"] = c.auth.session.SessionID
		log.Printf("[HME] Using SessionID (len=%d)", len(c.auth.session.SessionID))
	} else {
		log.Printf("[HME] WARNING: No SessionID available!")
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

	// Log cookies being sent for debugging
	parsedURL, _ := url.Parse(urlPath)
	cookies := c.auth.session.Client.Jar.Cookies(parsedURL)
	cookieNames := make([]string, 0, len(cookies))
	for _, ck := range cookies {
		cookieNames = append(cookieNames, ck.Name)
	}
	log.Printf("[HME] %s %s - sending cookies: %v", method, urlPath, cookieNames)

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

	// NOTE: We skip exchangeAccountSession() because browser doesn't do it after 2FA
	// The myacinfo cookie from 2FA is sufficient

	// Ensure myacinfo cookie exists
	c.ensureMyacinfo()

	// CRITICAL: Set idclient=web cookie on appleid.apple.com (browser does this)
	// This is required for token exchange to work
	appleidURL, _ := url.Parse("https://appleid.apple.com")
	c.auth.session.Client.Jar.SetCookies(appleidURL, []*http.Cookie{{
		Name:  "idclient",
		Value: "web",
		Path:  "/",
	}})
	log.Printf("[HME] Set idclient=web on appleid.apple.com")

	// Log cookies for debugging
	c.logCookies()

	// Step 1: Bootstrap portal
	resp, err := c.doRequest("GET", BootstrapURL, nil)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("[HME] bootstrap status=%d", resp.StatusCode)
	if resp.StatusCode != 200 {
		return fmt.Errorf("bootstrap failed: HTTP %d - %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	// Capture aidsp and other cookies from bootstrap response
	for _, cookie := range resp.Cookies() {
		log.Printf("[HME] bootstrap Set-Cookie: %s (domain=%s)", cookie.Name, cookie.Domain)
		if cookie.Name == "aidsp" {
			// Ensure aidsp is set on appleid.apple.com
			c.auth.session.Client.Jar.SetCookies(appleidURL, []*http.Cookie{{
				Name:  "aidsp",
				Value: cookie.Value,
				Path:  "/",
			}})
			log.Printf("[HME] Set aidsp on appleid.apple.com")
		}
	}

	// Step 2: Token exchange (non-fatal — HME API may work without it)
	resp, err = c.doRequest("GET", TokenURL, nil)
	if err != nil {
		log.Printf("[HME] token exchange error (non-fatal): %v", err)
	} else {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[HME] token exchange status=%d", resp.StatusCode)
		if resp.StatusCode != 200 {
			log.Printf("[HME] token exchange failed (non-fatal): HTTP %d", resp.StatusCode)
		}
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

	resp, err := c.doRequest("GET", AccountBase+"/forwardemail", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get forward emails failed: HTTP %d", resp.StatusCode)
	}

	var result ForwardEmailResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.ForwardToOptions.AvailableEmails, nil
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

// ExtendSession extends the session lifetime
func (c *HMEClient) ExtendSession() error {
	resp, err := c.doRequest("POST", "https://appleid.apple.com/session/extend", map[string]interface{}{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("extend session failed: HTTP %d", resp.StatusCode)
	}

	return nil
}

package apple

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

var srpLog *log.Logger

func init() {
	logPath := "srp_debug.log"
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		srpLog = log.New(os.Stderr, "[SRP] ", log.LstdFlags)
	} else {
		srpLog = log.New(f, "[SRP] ", log.LstdFlags)
	}
}

// AppleAuth handles Apple ID authentication
type AppleAuth struct {
	session   *Session
	srpClient *SRPClient
}

// NewAppleAuth creates a new auth client
func NewAppleAuth() *AppleAuth {
	return &AppleAuth{
		session: NewSession(),
	}
}

// GetSession returns the current session
func (a *AppleAuth) GetSession() *Session {
	return a.session
}

// generateOAuthState generates a random OAuth state string like browser does
func generateOAuthState() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(time.Nanosecond)
	}
	return "auth-" + string(b[:8]) + "-" + string(b[8:12]) + "-" + string(b[12:16]) + "-" + string(b[16:20]) + "-" + string(b[20:28])
}

// generateFDClientInfo generates device fingerprint JSON like browser
func generateFDClientInfo() string {
	return `{"U":"` + ChromeUA + `","L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":""}`
}

// common headers for auth requests - using account.apple.com config (not iCloud)
func (a *AppleAuth) authHeaders() map[string]string {
	a.session.mu.Lock()
	if a.session.OAuthState == "" {
		a.session.OAuthState = generateOAuthState()
	}
	oauthState := a.session.OAuthState
	a.session.mu.Unlock()

	headers := map[string]string{
		// Basic headers
		"Content-Type":   "application/json",
		"Accept":         "application/json, text/plain, */*",
		"User-Agent":     ChromeUA,
		"Origin":         "https://idmsa.apple.com",
		"Referer":        "https://idmsa.apple.com/",
		"Accept-Language": "zh-CN,zh;q=0.9",
		
		// OAuth headers - CRITICAL for myacinfo cookie
		"X-Apple-Widget-Key":         AuthWidgetKey,
		"X-Apple-OAuth-Client-Id":    AuthWidgetKey,
		"X-Apple-OAuth-Client-Type":  "firstPartyAuth",
		"X-Apple-OAuth-Redirect-URI": "https://account.apple.com",
		"X-Apple-OAuth-Response-Mode": "web_message",
		"X-Apple-OAuth-Response-Type": "code",
		"X-Apple-OAuth-State":        oauthState,
		"X-Apple-App-Id":             AuthWidgetKey,
		"X-Apple-Frame-Id":           oauthState,
		
		// Domain and privacy headers
		"X-Apple-Domain-Id":               "11",
		"X-Apple-Privacy-Consent":         "true",
		"X-Apple-Privacy-Consent-Accepted": "true",
		
		// Device fingerprint
		"X-Apple-I-FD-Client-Info": generateFDClientInfo(),
		
		// Security headers
		"Sec-Ch-Ua":          `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-origin",
	}
	
	a.session.mu.RLock()
	if a.session.SCNT != "" {
		headers["scnt"] = a.session.SCNT
	}
	if a.session.SessionID != "" {
		headers["X-Apple-ID-Session-Id"] = a.session.SessionID
	}
	if a.session.AuthAttributes != "" {
		headers["X-Apple-Auth-Attributes"] = a.session.AuthAttributes
	}
	a.session.mu.RUnlock()
	
	return headers
}

// doRequest performs an HTTP request with auth headers
func (a *AppleAuth) doRequest(method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range a.authHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := a.session.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// Capture session headers
	a.captureSessionHeaders(resp)

	return resp, nil
}

// captureSessionHeaders extracts session tokens from response
func (a *AppleAuth) captureSessionHeaders(resp *http.Response) {
	a.session.mu.Lock()
	defer a.session.mu.Unlock()

	if scnt := resp.Header.Get("scnt"); scnt != "" {
		a.session.SCNT = scnt
	}
	if sid := resp.Header.Get("X-Apple-ID-Session-Id"); sid != "" {
		a.session.SessionID = sid
	}
	if token := resp.Header.Get("X-Apple-Session-Token"); token != "" {
		a.session.SessionToken = token
	}
	// Capture auth attributes - CRITICAL for 2FA requests to return myacinfo
	if authAttr := resp.Header.Get("X-Apple-Auth-Attributes"); authAttr != "" {
		a.session.AuthAttributes = authAttr
		log.Printf("[CaptureHeaders] Got X-Apple-Auth-Attributes (len=%d)", len(authAttr))
	}
	a.session.LastActivity = time.Now()

	// CRITICAL: Capture myacinfo cookie and set it on ALL Apple domains
	// Go's cookie jar may not properly handle Domain=apple.com for subdomains
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "myacinfo" && cookie.Value != "" {
			log.Printf("[CaptureHeaders] Found myacinfo cookie (len=%d), setting on all Apple domains", len(cookie.Value))
			// Set on all relevant Apple domains
			domains := []string{
				"https://apple.com",
				"https://appleid.apple.com",
				"https://idmsa.apple.com",
				"https://account.apple.com",
			}
			for _, domain := range domains {
				u, _ := url.Parse(domain)
				a.session.Client.Jar.SetCookies(u, []*http.Cookie{{
					Name:  "myacinfo",
					Value: cookie.Value,
					Path:  "/",
				}})
			}
			log.Printf("[CaptureHeaders] myacinfo set on %d domains", len(domains))
		}
	}
}

// Federate performs federation check
func (a *AppleAuth) Federate(username string) error {
	url := AuthBase + "/federate?isRememberMeEnabled=true"
	body := map[string]interface{}{
		"accountName": username,
		"rememberMe":  true,
	}

	resp, err := a.doRequest("POST", url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// Login performs SRP login
func (a *AppleAuth) Login(username, password string) (*LoginResult, error) {
	// Step 1: Create SRP client
	srpClient, err := NewSRPClient(ModeGSA, HashSHA256, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to create SRP client: %w", err)
	}
	srpClient.SetIdentity(username)
	a.srpClient = srpClient

	// Step 2: SRP Init
	aB64 := base64.StdEncoding.EncodeToString(srpClient.GetPublicKey())
	initReq := SRPInitRequest{
		A:           aB64,
		AccountName: username,
		Protocols:   []string{"s2k", "s2k_fo"},
	}

	resp, err := a.doRequest("POST", AuthBase+"/signin/init", initReq)
	if err != nil {
		return nil, fmt.Errorf("SRP init failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SRP init failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var initResp SRPInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return nil, fmt.Errorf("failed to decode init response: %w", err)
	}
	srpLog.Printf("init ok: protocol=%s, iteration=%d, salt_len=%d, B_len=%d",
		initResp.Protocol, initResp.Iteration, len(initResp.Salt), len(initResp.B))

	// Step 3: Handle Hashcash challenge
	hcToken := ""
	if bits := resp.Header.Get("X-Apple-HC-Bits"); bits != "" {
		if challenge := resp.Header.Get("X-Apple-HC-Challenge"); challenge != "" {
			var bitCount int
			fmt.Sscanf(bits, "%d", &bitCount)
			hcToken = solveHashcash(bitCount, challenge)
		}
	}

	// Step 4: Derive password
	passHash := sha256.Sum256([]byte(password))
	var passKey []byte
	if initResp.Protocol == "s2k_fo" {
		passKey = []byte(hex.EncodeToString(passHash[:]))
	} else {
		passKey = passHash[:]
	}

	saltBytes, err := base64.StdEncoding.DecodeString(initResp.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	derivedKey := pbkdf2.Key(passKey, saltBytes, initResp.Iteration, 32, sha256.New)
	srpClient.SetPassword(derivedKey)
	srpLog.Printf("password derived: protocol=%s, key_len=%d, salt_hex=%s, dk_hex=%s",
		initResp.Protocol, len(derivedKey), hex.EncodeToString(saltBytes), hex.EncodeToString(derivedKey[:8]))

	// Step 5: Generate proof
	serverB, err := base64.StdEncoding.DecodeString(initResp.B)
	if err != nil {
		return nil, fmt.Errorf("failed to decode server B: %w", err)
	}

	m1Hex, err := srpClient.Generate(saltBytes, serverB)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proof: %w", err)
	}

	srpLog.Printf("M1_hex=%s", m1Hex)
	srpLog.Printf("K_hex=%s, A_len=%d, B_len=%d",
		hex.EncodeToString(srpClient.K[:8]), len(srpClient.A.Bytes()), len(serverB))
	m1Bytes, _ := hex.DecodeString(m1Hex)
	m1B64 := base64.StdEncoding.EncodeToString(m1Bytes)
	m2B64 := base64.StdEncoding.EncodeToString(srpClient.GenerateM2())

	// Step 6: Complete login
	completeReq := SRPCompleteRequest{
		AccountName: username,
		TrustTokens: []string{},
		RememberMe:  true,
		M1:          m1B64,
		M2:          m2B64,
		C:           initResp.C,
	}

	// Add Hashcash header if needed
	headers := a.authHeaders()
	if hcToken != "" {
		headers["X-Apple-HC"] = hcToken
	}

	jsonData, _ := json.Marshal(completeReq)
	req, _ := http.NewRequest("POST", AuthBase+"/signin/complete?isRememberMeEnabled=true", bytes.NewReader(jsonData))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	srpLog.Printf("sending complete: m1_len=%d, m2_len=%d", len(m1B64), len(m2B64))
	resp, err = a.session.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("complete request failed: %w", err)
	}
	defer resp.Body.Close()
	a.captureSessionHeaders(resp)
	srpLog.Printf("complete response: status=%d", resp.StatusCode)

	switch resp.StatusCode {
	case 200:
		a.session.mu.Lock()
		a.session.Authenticated = true
		a.session.AppleID = username
		a.session.mu.Unlock()
		return &LoginResult{State: AuthStateAuthenticated, Message: "Login successful"}, nil

	case 409:
		// Parse phone numbers from 409 response
		var twoFAInfo struct {
			TrustedPhoneNumbers []struct {
				ID                 int    `json:"id"`
				NumberWithDialCode string `json:"numberWithDialCode"`
				ObfuscatedNumber   string `json:"obfuscatedNumber"`
				PushMode           string `json:"pushMode"`
			} `json:"trustedPhoneNumbers"`
		}
		resBody, _ := io.ReadAll(resp.Body)
		json.Unmarshal(resBody, &twoFAInfo)
		srpLog.Printf("409 2FA info: %s", string(resBody))

		result := &LoginResult{State: AuthStateNeeds2FA, Requires2FA: true, Message: "2FA required"}
		for _, p := range twoFAInfo.TrustedPhoneNumbers {
			number := p.NumberWithDialCode
			if number == "" {
				number = p.ObfuscatedNumber
			}
			result.PhoneNumbers = append(result.PhoneNumbers, struct {
				ID                 int    `json:"id"`
				NumberWithDialCode string `json:"numberWithDialCode"`
			}{ID: p.ID, NumberWithDialCode: number})
		}
		return result, nil

	case 401:
		respBody, _ := io.ReadAll(resp.Body)
		srpLog.Printf("401 response body: %s", string(respBody))
		return nil, fmt.Errorf("SRP verification failed: %s", string(respBody))

	case 403:
		return nil, fmt.Errorf("wrong password or account locked")

	case 412:
		return nil, fmt.Errorf("additional action required on Apple device")

	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login failed: HTTP %d - %s", resp.StatusCode, string(body))
	}
}

// Verify2FADevice verifies 2FA code from trusted device
func (a *AppleAuth) Verify2FADevice(code string) error {
	req := TwoFARequest{
		SecurityCode: SecurityCode{Code: code},
	}

	resp, err := a.doRequest("POST", AuthBase+"/verify/trusteddevice/securitycode", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		// Trust the device
		a.trustDevice()
		a.session.mu.Lock()
		a.session.Authenticated = true
		a.session.mu.Unlock()
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("2FA verification failed: HTTP %d - %s", resp.StatusCode, string(body))
}

// Verify2FASMS verifies 2FA code from SMS
func (a *AppleAuth) Verify2FASMS(phoneID int, code string) error {
	req := TwoFASMSRequest{
		SecurityCode: SecurityCode{Code: code},
		PhoneNumber:  PhoneNumber{ID: phoneID},
		Mode:         "sms",
	}

	resp, err := a.doRequest("POST", AuthBase+"/verify/phone/securitycode", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		a.trustDevice()
		a.session.mu.Lock()
		a.session.Authenticated = true
		a.session.mu.Unlock()
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("SMS 2FA verification failed: HTTP %d - %s", resp.StatusCode, string(body))
}

// RequestSMSCode requests a new SMS code
func (a *AppleAuth) RequestSMSCode(phoneID int) error {
	req := map[string]interface{}{
		"phoneNumber": map[string]int{"id": phoneID},
		"mode":        "sms",
	}

	resp, err := a.doRequest("PUT", AuthBase+"/verify/phone", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		return nil
	}

	return fmt.Errorf("failed to request SMS code: HTTP %d", resp.StatusCode)
}

// trustDevice calls trust endpoint
func (a *AppleAuth) trustDevice() {
	resp, err := a.doRequest("GET", AuthBase+"/2sv/trust", nil)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// IsAuthenticated checks if session is authenticated
func (a *AppleAuth) IsAuthenticated() bool {
	a.session.mu.RLock()
	defer a.session.mu.RUnlock()
	return a.session.Authenticated
}

// solveHashcash solves Apple's hashcash challenge
func solveHashcash(bits int, challenge string) string {
	dateStr := time.Now().UTC().Format("060102150405")
	prefix := fmt.Sprintf("1:%d:%s:%s::", bits, dateStr, challenge)
	
	checkBytes := (bits >> 3) + 1
	shift := uint(checkBytes*8 - bits)
	
	counter := 0
	for {
		stamp := fmt.Sprintf("%s%d", prefix, counter)
		hash := sha1.Sum([]byte(stamp))
		
		val := uint32(0)
		for i := 0; i < checkBytes; i++ {
			val = (val << 8) | uint32(hash[i])
		}
		
		if val>>shift == 0 {
			return stamp
		}
		counter++
	}
}

// SerializableCookie represents a cookie for JSON serialization
type SerializableCookie struct {
	Name   string `json:"n"`
	Value  string `json:"v"`
	Domain string `json:"d"`
	Path   string `json:"p"`
}

var cookieDomains = []string{
	"https://apple.com",
	"https://idmsa.apple.com",
	"https://appleid.apple.com",
	"https://account.apple.com",
	"https://www.icloud.com",
	"https://setup.icloud.com",
}

// ExportSessionData exports the full session state for persistence
func (a *AppleAuth) ExportSessionData() (token, scnt, sessionID, cookiesJSON string) {
	a.session.mu.RLock()
	defer a.session.mu.RUnlock()

	// Use myacinfo as token if SessionToken is empty
	token = a.session.SessionToken
	if token == "" {
		// Try to get myacinfo cookie as fallback token
		u, _ := url.Parse("https://apple.com")
		for _, c := range a.session.Client.Jar.Cookies(u) {
			if c.Name == "myacinfo" && c.Value != "" {
				token = c.Value
				break
			}
		}
	}
	scnt = a.session.SCNT
	sessionID = a.session.SessionID

	var allCookies []SerializableCookie
	for _, d := range cookieDomains {
		u, _ := url.Parse(d)
		for _, c := range a.session.Client.Jar.Cookies(u) {
			allCookies = append(allCookies, SerializableCookie{
				Name:   c.Name,
				Value:  c.Value,
				Domain: u.Host,
				Path:   "/",
			})
		}
	}
	data, _ := json.Marshal(allCookies)
	cookiesJSON = string(data)
	return
}

// RestoreAppleAuth restores a full AppleAuth from persisted session data
func RestoreAppleAuth(token, scnt, sessionID, cookiesJSON string) *AppleAuth {
	auth := NewAppleAuth()
	auth.session.mu.Lock()
	auth.session.SessionToken = token
	auth.session.SCNT = scnt
	auth.session.SessionID = sessionID
	auth.session.Authenticated = true
	auth.session.LastActivity = time.Now()

	// Restore cookies into the jar
	if cookiesJSON != "" {
		var cookies []SerializableCookie
		if json.Unmarshal([]byte(cookiesJSON), &cookies) == nil {
			// Group cookies by domain
			byDomain := make(map[string][]*http.Cookie)
			for _, c := range cookies {
				byDomain[c.Domain] = append(byDomain[c.Domain], &http.Cookie{
					Name:  c.Name,
					Value: c.Value,
					Path:  c.Path,
				})
			}
			for domain, cs := range byDomain {
				u, _ := url.Parse("https://" + domain)
				auth.session.Client.Jar.SetCookies(u, cs)
			}
		}
	}

	auth.session.mu.Unlock()
	return auth
}

// ExportCookies exports session cookies as string (for debugging)
func (a *AppleAuth) ExportCookies() string {
	a.session.mu.RLock()
	defer a.session.mu.RUnlock()

	var parts []string
	for _, d := range cookieDomains {
		u, _ := url.Parse(d)
		for _, cookie := range a.session.Client.Jar.Cookies(u) {
			parts = append(parts, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
		}
	}
	return strings.Join(parts, "; ")
}

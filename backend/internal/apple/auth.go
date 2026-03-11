package apple

import (
	"bytes"
	"crypto/rand"
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
var debugMode bool

// SetDebugMode enables/disables verbose SRP and auth logging
func SetDebugMode(enabled bool) {
	debugMode = enabled
}

var authDebugFile *os.File

func init() {
	// Default: discard SRP debug logs; enable via SetDebugMode(true)
	srpLog = log.New(io.Discard, "[SRP] ", log.LstdFlags)
}

// logAuthRequest logs detailed request info to auth_debug.log
func logAuthRequest(method, urlStr string, headers map[string]string, body []byte, jar http.CookieJar) {
	if authDebugFile == nil {
		var err error
		authDebugFile, err = os.OpenFile("auth_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("[AuthDebug] Failed to open log file: %v", err)
			return
		}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(authDebugFile, "\n========== %s ==========\n", timestamp)
	fmt.Fprintf(authDebugFile, "REQUEST: %s %s\n", method, urlStr)
	
	fmt.Fprintf(authDebugFile, "\n--- Request Headers ---\n")
	for k, v := range headers {
		// Truncate long values for readability
		if len(v) > 200 {
			fmt.Fprintf(authDebugFile, "%s: %s... (len=%d)\n", k, v[:200], len(v))
		} else {
			fmt.Fprintf(authDebugFile, "%s: %s\n", k, v)
		}
	}
	
	if body != nil {
		fmt.Fprintf(authDebugFile, "\n--- Request Body ---\n%s\n", string(body))
	}
	
	// Log cookies for the request URL
	if jar != nil {
		u, _ := url.Parse(urlStr)
		if u != nil {
			cookies := jar.Cookies(u)
			if len(cookies) > 0 {
				fmt.Fprintf(authDebugFile, "\n--- Cookies for %s ---\n", u.Host)
				for _, c := range cookies {
					if len(c.Value) > 50 {
						fmt.Fprintf(authDebugFile, "%s: %s... (len=%d)\n", c.Name, c.Value[:50], len(c.Value))
					} else {
						fmt.Fprintf(authDebugFile, "%s: %s\n", c.Name, c.Value)
					}
				}
			}
		}
	}
	
	authDebugFile.Sync()
}

// LogAuthResponse logs detailed response info to auth_debug.log
func LogAuthResponse(statusCode int, headers http.Header, body []byte) {
	if authDebugFile == nil {
		return
	}
	
	fmt.Fprintf(authDebugFile, "\n--- Response Status ---\n%d\n", statusCode)
	
	fmt.Fprintf(authDebugFile, "\n--- Response Headers ---\n")
	for k, vals := range headers {
		for _, v := range vals {
			if len(v) > 200 {
				fmt.Fprintf(authDebugFile, "%s: %s... (len=%d)\n", k, v[:200], len(v))
			} else {
				fmt.Fprintf(authDebugFile, "%s: %s\n", k, v)
			}
		}
	}
	
	if body != nil {
		fmt.Fprintf(authDebugFile, "\n--- Response Body ---\n%s\n", string(body))
	}
	
	fmt.Fprintf(authDebugFile, "\n=====================================\n")
	authDebugFile.Sync()
}

func enableSRPLog() {
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

// generateOAuthState generates a cryptographically random OAuth state string
func generateOAuthState() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 28)
	randBytes := make([]byte, 28)
	if _, err := rand.Read(randBytes); err != nil {
		// Fallback — should never happen
		for i := range b {
			b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		}
	} else {
		for i := range b {
			b[i] = chars[int(randBytes[i])%len(chars)]
		}
	}
	return "auth-" + string(b[:8]) + "-" + string(b[8:12]) + "-" + string(b[12:16]) + "-" + string(b[16:20]) + "-" + string(b[20:28])
}

// generateFDClientInfo generates device fingerprint JSON like browser
// The F field is a browser fingerprint that Apple uses for fraud detection
func generateFDClientInfo() string {
	// Generate a random-ish but consistent fingerprint
	// Format matches what Chrome sends: base64-encoded fingerprint data
	fp := generateBrowserFingerprint()
	return `{"U":"` + ChromeUA + `","L":"zh-CN","Z":"GMT+08:00","V":"1.1","F":"` + fp + `"}`
}

// generateBrowserFingerprint creates a browser fingerprint similar to what Apple's JS generates
func generateBrowserFingerprint() string {
	// This mimics the fingerprint format Apple expects
	// Real browsers generate this from canvas, webgl, fonts, etc.
	// We use a realistic-looking static fingerprint that changes slightly per session
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	
	// Build a fingerprint string similar to browser format
	// Format: "Fla44j1e3NlY5BNlY5BSmHACVZXnNAA9..." 
	base := "Fla44j1e3NlY5BNlY5BSmHACVZXnNAA9Zdd"
	mid := fmt.Sprintf("%x", randBytes[:4])
	tail := "azLu_dYV6Hycfx9MsFY5Bhw.Tf5.EKWJ9V68DA2uZjkmjsTclY5BNleBBNlYCa1nkBMfs"
	randTail := fmt.Sprintf(".%x", randBytes[4:6])
	
	return base + mid + tail + randTail
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
func (a *AppleAuth) doRequest(method, urlStr string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, err
	}

	headers := a.authHeaders()
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Debug logging to file - request
	if debugMode {
		logAuthRequest(method, urlStr, headers, bodyBytes, a.session.Client.Jar)
	}

	resp, err := a.session.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// Capture session headers
	a.captureSessionHeaders(resp)

	// Debug logging to file - response (read and re-wrap body)
	if debugMode {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		LogAuthResponse(resp.StatusCode, resp.Header, respBody)
		// Re-wrap body so caller can still read it
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	}

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
		if debugMode {
			log.Printf("[CaptureHeaders] Got X-Apple-Auth-Attributes (len=%d)", len(authAttr))
		}
	}
	a.session.LastActivity = time.Now()

	// CRITICAL: Capture myacinfo cookie and set it on ALL Apple domains
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "myacinfo" && cookie.Value != "" {
			if debugMode {
				log.Printf("[CaptureHeaders] Found myacinfo cookie (len=%d)", len(cookie.Value))
			}
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
	completeURL := AuthBase + "/signin/complete?isRememberMeEnabled=true"
	req, _ := http.NewRequest("POST", completeURL, bytes.NewReader(jsonData))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Debug logging
	if debugMode {
		logAuthRequest("POST", completeURL, headers, jsonData, a.session.Client.Jar)
	}

	srpLog.Printf("sending complete: m1_len=%d, m2_len=%d", len(m1B64), len(m2B64))
	resp, err = a.session.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("complete request failed: %w", err)
	}
	defer resp.Body.Close()
	a.captureSessionHeaders(resp)
	
	// Read and log response body
	respBody, _ := io.ReadAll(resp.Body)
	if debugMode {
		LogAuthResponse(resp.StatusCode, resp.Header, respBody)
	}
	srpLog.Printf("complete response: status=%d, body=%s", resp.StatusCode, string(respBody))

	switch resp.StatusCode {
	case 200:
		a.session.mu.Lock()
		a.session.Authenticated = true
		a.session.AppleID = username
		a.session.mu.Unlock()
		return &LoginResult{State: AuthStateAuthenticated, Message: "Login successful"}, nil

	case 409:
		// 409 means 2FA required - need to call GET /auth to get phone numbers
		srpLog.Printf("409 2FA required, calling GET /auth to get phone numbers")
		
		// Call GET /auth to get trustedPhoneNumbers
		authResp, err := a.doRequest("GET", AuthBase, nil)
		if err != nil {
			log.Printf("[Login] Failed to get auth state: %v", err)
			return &LoginResult{State: AuthStateNeeds2FA, Requires2FA: true, Message: "2FA required"}, nil
		}
		defer authResp.Body.Close()
		
		authBody, _ := io.ReadAll(authResp.Body)
		var twoFAInfo struct {
			TrustedPhoneNumbers []struct {
				ID                 int    `json:"id"`
				NumberWithDialCode string `json:"numberWithDialCode"`
				ObfuscatedNumber   string `json:"obfuscatedNumber"`
				PushMode           string `json:"pushMode"`
			} `json:"trustedPhoneNumbers"`
		}
		json.Unmarshal(authBody, &twoFAInfo)
		srpLog.Printf("GET /auth returned %d phone numbers", len(twoFAInfo.TrustedPhoneNumbers))

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
		log.Printf("[Login] 2FA required, phoneNumbers=%+v", result.PhoneNumbers)
		return result, nil

	case 401:
		srpLog.Printf("401 response body: %s", string(respBody))
		return nil, fmt.Errorf("SRP verification failed: %s", string(respBody))

	case 403:
		return nil, fmt.Errorf("wrong password or account locked")

	case 412:
		return nil, fmt.Errorf("additional action required on Apple device")

	default:
		return nil, fmt.Errorf("login failed: HTTP %d - %s", resp.StatusCode, string(respBody))
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

// GetAuthState calls GET /auth to refresh session state before SMS request
// This is critical - browser always calls this after login returns 409
func (a *AppleAuth) GetAuthState() (int, error) {
	resp, err := a.doRequest("GET", AuthBase, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	// Read body to ensure cookies are captured
	io.ReadAll(resp.Body)
	
	log.Printf("[GetAuthState] status=%d", resp.StatusCode)
	return resp.StatusCode, nil
}

// RequestSMSCode requests a new SMS code
func (a *AppleAuth) RequestSMSCode(phoneID int) error {
	// CRITICAL: Call GET /auth first to refresh session state and get crsc cookie
	authStatus, err := a.GetAuthState()
	if err != nil {
		log.Printf("[RequestSMSCode] GetAuthState error: %v", err)
	}
	
	// 423 = account locked/needs device verification
	if authStatus == 423 {
		return fmt.Errorf("账户被锁定或需要在Apple设备上验证，请先在浏览器登录 account.apple.com 完成验证")
	}
	
	req := map[string]interface{}{
		"phoneNumber": map[string]int{"id": phoneID},
		"mode":        "sms",
	}

	resp, err := a.doRequest("PUT", AuthBase+"/verify/phone", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		return nil
	}

	switch resp.StatusCode {
	case 412:
		return fmt.Errorf("请求被拒绝(412)，该账户可能被 Apple 风控限制，请先在浏览器登录一次")
	case 423:
		return fmt.Errorf("账户被锁定(423)，请在Apple设备上解锁后重试")
	case 429:
		return fmt.Errorf("请求过于频繁(429)，请稍后重试")
	default:
		return fmt.Errorf("发送验证码失败: HTTP %d - %s", resp.StatusCode, string(body))
	}
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
	var cookieNames []string
	keyCookieFound := make(map[string]bool) // track key cookies
	for _, d := range cookieDomains {
		u, _ := url.Parse(d)
		for _, c := range a.session.Client.Jar.Cookies(u) {
			allCookies = append(allCookies, SerializableCookie{
				Name:   c.Name,
				Value:  c.Value,
				Domain: u.Host,
				Path:   "/",
			})
			cookieNames = append(cookieNames, u.Host+":"+c.Name)
			// Track key cookies
			if c.Name == "myacinfo" || c.Name == "aidsp" || c.Name == "acn01" {
				keyCookieFound[c.Name] = true
			}
		}
	}
	data, _ := json.Marshal(allCookies)
	cookiesJSON = string(data)

	// Log key cookie status
	log.Printf("[Export] Key cookies: myacinfo=%v, aidsp=%v, acn01=%v",
		keyCookieFound["myacinfo"], keyCookieFound["aidsp"], keyCookieFound["acn01"])
	log.Printf("[Export] token_len=%d, scnt_len=%d, sessionID_len=%d, cookies=%d",
		len(token), len(scnt), len(sessionID), len(allCookies))
	return
}

// RestoreAppleAuth restores a full AppleAuth from persisted session data
func RestoreAppleAuth(token, scnt, sessionID, cookiesJSON string) *AppleAuth {
	log.Printf("[Restore] Input: token_len=%d, scnt_len=%d, sessionID_len=%d, cookiesJSON_len=%d",
		len(token), len(scnt), len(sessionID), len(cookiesJSON))

	auth := NewAppleAuth()
	auth.session.mu.Lock()
	auth.session.SessionToken = token
	auth.session.SCNT = scnt
	auth.session.SessionID = sessionID
	auth.session.Authenticated = true
	auth.session.LastActivity = time.Now()

	// Restore cookies into the jar
	var restoredNames []string
	if cookiesJSON != "" {
		var cookies []SerializableCookie
		if json.Unmarshal([]byte(cookiesJSON), &cookies) == nil {
			// Deduplicate cookies by domain+name (keep last occurrence)
			seen := make(map[string]bool)
			var uniqueCookies []SerializableCookie
			for i := len(cookies) - 1; i >= 0; i-- {
				c := cookies[i]
				key := c.Domain + "|" + c.Name
				if !seen[key] {
					seen[key] = true
					uniqueCookies = append([]SerializableCookie{c}, uniqueCookies...)
				}
			}

			// CRITICAL: Set cookies on ALL relevant Apple domains to ensure subdomain coverage
			// Apple's cookies need to be available across all subdomains
			allDomains := []string{
				"https://apple.com",
				"https://idmsa.apple.com",
				"https://appleid.apple.com",
				"https://account.apple.com",
				"https://www.icloud.com",
				"https://setup.icloud.com",
			}

			// Build a map of cookie name -> value from uniqueCookies
			cookieMap := make(map[string]string)
			for _, c := range uniqueCookies {
				cookieMap[c.Name] = c.Value
				restoredNames = append(restoredNames, c.Domain+":"+c.Name)
			}

			// Log key cookie status
			log.Printf("[Restore] Key cookies found: myacinfo=%v (len=%d), aidsp=%v (len=%d), acn01=%v (len=%d)",
				cookieMap["myacinfo"] != "", len(cookieMap["myacinfo"]),
				cookieMap["aidsp"] != "", len(cookieMap["aidsp"]),
				cookieMap["acn01"] != "", len(cookieMap["acn01"]))

			// Key cookies that must be set on all Apple domains
			keyCookies := []string{"myacinfo", "aidsp", "dslang", "site", "acn01"}

			// Set key cookies on all domains
			for _, cookieName := range keyCookies {
				if val, ok := cookieMap[cookieName]; ok && val != "" {
					for _, d := range allDomains {
						u, _ := url.Parse(d)
						auth.session.Client.Jar.SetCookies(u, []*http.Cookie{{
							Name:  cookieName,
							Value: val,
							Path:  "/",
						}})
					}
				}
			}

			// Also restore other cookies to their original domain
			for _, c := range uniqueCookies {
				u, _ := url.Parse("https://" + c.Domain)
				auth.session.Client.Jar.SetCookies(u, []*http.Cookie{{
					Name:  c.Name,
					Value: c.Value,
					Path:  c.Path,
				}})
			}
			log.Printf("[Restore] Restored %d unique cookies", len(uniqueCookies))
		}
	}

	// Also ensure myacinfo is set on all key domains if we have a token
	if token != "" {
		for _, d := range []string{"https://apple.com", "https://appleid.apple.com", "https://idmsa.apple.com", "https://account.apple.com"} {
			u, _ := url.Parse(d)
			auth.session.Client.Jar.SetCookies(u, []*http.Cookie{{
				Name:  "myacinfo",
				Value: token,
				Path:  "/",
			}})
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

// LogAllCookies logs all cookies across all Apple domains for debugging
func (a *AppleAuth) LogAllCookies() {
	a.session.mu.RLock()
	defer a.session.mu.RUnlock()

	for _, d := range cookieDomains {
		u, _ := url.Parse(d)
		cookies := a.session.Client.Jar.Cookies(u)
		if len(cookies) > 0 {
			var names []string
			for _, c := range cookies {
				// Truncate value for logging
				val := c.Value
				if len(val) > 20 {
					val = val[:20] + "..."
				}
				names = append(names, fmt.Sprintf("%s=%s", c.Name, val))
			}
			srpLog.Printf("  [%s] %v", u.Host, names)
		}
	}
	srpLog.Printf("  SessionToken=%v, SCNT=%v, SessionID=%v",
		a.session.SessionToken != "", a.session.SCNT != "", a.session.SessionID != "")
}

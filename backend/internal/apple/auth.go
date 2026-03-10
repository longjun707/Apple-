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
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

var srpLog *log.Logger

func init() {
	// 使用绝对路径确保热更新时日志写入正确位置
	logPath := `D:\DM\apple隐私邮箱创建\apple-hme-manager\backend\srp_debug.log`
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

// iCloudOAuthKey is the iCloud.com OAuth client ID used for SRP auth
// This matches the working Python implementation
const iCloudOAuthKey = "d39ba9916b7251055b22c7f910e2ea796ee65e98b2ddecea8f5dde8d9d1a815d"

// common headers for auth requests
func (a *AppleAuth) authHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type":                      "application/json",
		"Accept":                            "application/json, text/javascript",
		"User-Agent":                        ChromeUA,
		"X-Apple-OAuth-Client-Id":           iCloudOAuthKey,
		"X-Apple-OAuth-Client-Type":         "firstPartyAuth",
		"X-Apple-OAuth-Redirect-URI":        "https://www.icloud.com",
		"X-Apple-OAuth-Require-Grant-Code":  "true",
		"X-Apple-OAuth-Response-Mode":       "web_message",
		"X-Apple-OAuth-Response-Type":       "code",
		"X-Apple-Widget-Key":                iCloudOAuthKey,
		"X-Requested-With":                  "XMLHttpRequest",
		"Origin":                            "https://www.icloud.com",
		"Referer":                           "https://www.icloud.com/",
	}
	
	a.session.mu.RLock()
	if a.session.SCNT != "" {
		headers["scnt"] = a.session.SCNT
	}
	if a.session.SessionID != "" {
		headers["X-Apple-ID-Session-Id"] = a.session.SessionID
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
	a.session.LastActivity = time.Now()
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

// ExportCookies exports session cookies as string
func (a *AppleAuth) ExportCookies() string {
	a.session.mu.RLock()
	defer a.session.mu.RUnlock()

	var parts []string
	for _, cookie := range a.session.Client.Jar.Cookies(nil) {
		parts = append(parts, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	return strings.Join(parts, "; ")
}

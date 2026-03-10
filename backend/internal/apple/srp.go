package apple

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math/big"
	"os"
	"path/filepath"
)

// Hash types
type HashType int

const (
	HashSHA1 HashType = iota
	HashSHA256
	HashSHA384
	HashSHA512
)

// SRP Mode
type SRPMode int

const (
	ModeRFC2945 SRPMode = iota
	ModeSRPTools
	ModeGoSRP
	ModeGSA // Apple GSA mode
)

// PrimeField represents SRP prime field parameters
type PrimeField struct {
	G *big.Int // Generator
	N *big.Int // Prime modulus
	n int      // Byte length
}

// SRP client state
type SRPClient struct {
	mode SRPMode
	hash HashType
	pf   *PrimeField
	i    []byte   // Identity (username hash)
	p    []byte   // Password (derived key)
	a    *big.Int // Private ephemeral
	A    *big.Int // Public ephemeral
	k    *big.Int // Multiplier
	K    []byte   // Session key
	M    []byte   // Client proof
}

var knownPrimes = make(map[int]*PrimeField)

func init() {
	// Try to load primes from file
	execPath, _ := os.Executable()
	primesPath := filepath.Join(filepath.Dir(execPath), "primes.json")
	
	// Also try current directory
	if _, err := os.Stat(primesPath); os.IsNotExist(err) {
		primesPath = "primes.json"
	}
	
	data, err := os.ReadFile(primesPath)
	if err != nil {
		// Use embedded 2048-bit prime as fallback
		N2048, _ := new(big.Int).SetString("AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310DCD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF747359D041D5C33EA71D281E446B14773BCA97B43A23FB801676BD207A436C6481F1D2B9078717461A5B9D32E688F87748544523B524B0D57D5EA77A2775D2ECFA032CFBDBF52FB3786160279004E57AE6AF874E7303CE53299CCC041C7BC308D82A5698F3A8D0C38271AE35F8E9DBFBB694B5C803D89F7AE435DE236D525F54759B65E372FCD68EF20FA7111F9E4AFF73", 16)
		knownPrimes[2048] = &PrimeField{G: big.NewInt(2), N: N2048, n: 256}
		return
	}

	var primeData map[string]struct {
		G int    `json:"g"`
		N string `json:"N"`
		n int    `json:"n"`
	}
	if err := json.Unmarshal(data, &primeData); err != nil {
		return
	}

	for bits, p := range primeData {
		var bitSize int
		fmt.Sscanf(bits, "%d", &bitSize)
		N, ok := new(big.Int).SetString(p.N, 16)
		if !ok {
			continue
		}
		knownPrimes[bitSize] = &PrimeField{
			G: big.NewInt(int64(p.G)),
			N: N,
			n: p.n,
		}
	}
}

// NewSRPClient creates a new SRP client
func NewSRPClient(mode SRPMode, hashType HashType, bits int) (*SRPClient, error) {
	pf, ok := knownPrimes[bits]
	if !ok {
		return nil, fmt.Errorf("unsupported prime field size: %d", bits)
	}

	// Generate random private ephemeral a
	aBytes := make([]byte, pf.n)
	if _, err := rand.Read(aBytes); err != nil {
		return nil, err
	}
	a := new(big.Int).SetBytes(aBytes)

	// Calculate public ephemeral A = g^a mod N
	A := new(big.Int).Exp(pf.G, a, pf.N)

	// Calculate multiplier k = H(N || pad(g))
	gPadded := padTo(pf.G.Bytes(), pf.n)
	kBytes := hashBytes(hashType, append(pf.N.Bytes(), gPadded...))
	k := new(big.Int).SetBytes(kBytes)

	return &SRPClient{
		mode: mode,
		hash: hashType,
		pf:   pf,
		a:    a,
		A:    A,
		k:    k,
	}, nil
}

// SetIdentity sets the username for the SRP client
func (c *SRPClient) SetIdentity(username string) {
	if c.mode == ModeGoSRP {
		c.i = hashBytes(c.hash, []byte(username))
	} else {
		c.i = []byte(username)
	}
}

// SetPassword sets the derived password key
func (c *SRPClient) SetPassword(derivedKey []byte) {
	c.p = derivedKey
}

// GetPublicKey returns the client's public ephemeral value A as bytes
func (c *SRPClient) GetPublicKey() []byte {
	return c.A.Bytes()
}

// Generate calculates the client proof M1
func (c *SRPClient) Generate(salt, serverPublicKey []byte) (string, error) {
	B := new(big.Int).SetBytes(serverPublicKey)

	// Validate B
	if new(big.Int).Mod(B, c.pf.N).Sign() == 0 {
		return "", fmt.Errorf("invalid server public key")
	}

	// Calculate u = H(pad(A) || pad(B))
	aPadded := padTo(c.A.Bytes(), c.pf.n)
	bPadded := padTo(B.Bytes(), c.pf.n)
	uBytes := hashBytes(c.hash, append(aPadded, bPadded...))
	u := new(big.Int).SetBytes(uBytes)

	if u.Sign() == 0 {
		return "", fmt.Errorf("invalid server public key (u=0)")
	}

	// Calculate x based on mode
	var x *big.Int
	if c.mode == ModeGoSRP {
		xBytes := hashBytes(c.hash, concat(c.i, c.p, salt))
		x = new(big.Int).SetBytes(xBytes)
	} else {
		var inner []byte
		if c.mode == ModeGSA {
			inner = concat([]byte{}, []byte{0x3A}, c.p)
		} else {
			inner = concat(c.i, []byte{0x3A}, c.p)
		}
		innerHash := hashBytes(c.hash, inner)
		xBytes := hashBytes(c.hash, append(salt, innerHash...))
		x = new(big.Int).SetBytes(xBytes)
	}

	// S = (B - k * g^x) ^ (a + u * x) mod N
	gx := new(big.Int).Exp(c.pf.G, x, c.pf.N)
	kgx := new(big.Int).Mul(c.k, gx)
	t1 := new(big.Int).Sub(B, kgx)
	t1.Mod(t1, c.pf.N)

	ux := new(big.Int).Mul(u, x)
	t2 := new(big.Int).Add(c.a, ux)

	S := new(big.Int).Exp(t1, t2, c.pf.N)

	// K = H(S)
	if c.mode == ModeRFC2945 {
		c.K = hashInterleave(c.hash, S.Bytes())
	} else {
		c.K = hashBytes(c.hash, S.Bytes())
	}

	// Calculate M1
	if c.mode == ModeGoSRP {
		c.M = hashBytes(c.hash, concat(
			c.K,
			c.A.Bytes(),
			B.Bytes(),
			c.i,
			salt,
			c.pf.N.Bytes(),
			c.pf.G.Bytes(),
		))
	} else {
		hN := hashBytes(c.hash, c.pf.N.Bytes())
		var hg []byte
		if c.mode == ModeGSA {
			hg = hashBytes(c.hash, padTo(c.pf.G.Bytes(), c.pf.n))
		} else {
			hg = hashBytes(c.hash, c.pf.G.Bytes())
		}
		xorNG := xorBytes(hN, hg)
		hI := hashBytes(c.hash, c.i)

		c.M = hashBytes(c.hash, concat(
			xorNG,
			hI,
			salt,
			c.A.Bytes(),
			B.Bytes(),
			c.K,
		))
	}

	return hex.EncodeToString(c.M), nil
}

// GenerateM2 generates the M2 value for server verification
func (c *SRPClient) GenerateM2() []byte {
	return hashBytes(c.hash, concat(c.A.Bytes(), c.M, c.K))
}

// VerifyServer verifies the server's proof
func (c *SRPClient) VerifyServer(serverProof string) bool {
	expected := c.GenerateM2()
	proof, err := hex.DecodeString(serverProof)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(expected, proof) == 1
}

// Helper functions
func hashBytes(hashType HashType, data []byte) []byte {
	var h hash.Hash
	switch hashType {
	case HashSHA1:
		h = sha1.New()
	case HashSHA256:
		h = sha256.New()
	case HashSHA384:
		h = sha512.New384()
	case HashSHA512:
		h = sha512.New()
	default:
		h = sha256.New()
	}
	h.Write(data)
	return h.Sum(nil)
}

func padTo(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	padded := make([]byte, n)
	copy(padded[n-len(b):], b)
	return padded
}

func concat(parts ...[]byte) []byte {
	var total int
	for _, p := range parts {
		total += len(p)
	}
	result := make([]byte, 0, total)
	for _, p := range parts {
		result = append(result, p...)
	}
	return result
}

func xorBytes(a, b []byte) []byte {
	if len(a) != len(b) {
		return nil
	}
	result := make([]byte, len(a))
	for i := range a {
		result[i] = a[i] ^ b[i]
	}
	return result
}

func hashInterleave(hashType HashType, data []byte) []byte {
	// Skip leading zeros and align to even length
	start := 0
	for start < len(data) && data[start] == 0 {
		start++
	}
	if (len(data)-start)%2 == 1 {
		start++
	}
	data = data[start:]

	// Split into even and odd bytes
	halfLen := len(data) / 2
	even := make([]byte, halfLen)
	odd := make([]byte, halfLen)
	for i := 0; i < halfLen; i++ {
		even[i] = data[i*2]
		odd[i] = data[i*2+1]
	}

	h1 := hashBytes(hashType, even)
	h2 := hashBytes(hashType, odd)

	// Interleave results
	result := make([]byte, len(h1)*2)
	for i := 0; i < len(h1); i++ {
		result[i*2] = h1[i]
		result[i*2+1] = h2[i]
	}
	return result
}

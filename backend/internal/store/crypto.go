package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const encryptedPrefix = "enc:"

var encryptionKey []byte

// InitEncryption initializes the encryption key from environment
func InitEncryption() {
	key := os.Getenv("ENCRYPT_KEY")
	if key == "" {
		log.Println("⚠️  ENCRYPT_KEY not set — Apple passwords will be stored unencrypted!")
		return
	}
	// Derive a 32-byte key using SHA-256
	hash := sha256.Sum256([]byte(key))
	encryptionKey = hash[:]
	log.Println("🔒 Encryption key loaded")
}

// EncryptPassword encrypts a password using AES-GCM.
// Returns the original string if encryption is not configured.
func EncryptPassword(plaintext string) string {
	if len(encryptionKey) == 0 || plaintext == "" {
		return plaintext
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		log.Printf("⚠️  Encryption failed (cipher): %v", err)
		return plaintext
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("⚠️  Encryption failed (gcm): %v", err)
		return plaintext
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Printf("⚠️  Encryption failed (nonce): %v", err)
		return plaintext
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext)
}

// DecryptPassword decrypts an AES-GCM encrypted password.
// If the value is not encrypted (no prefix), returns as-is for backward compatibility.
func DecryptPassword(stored string) (string, error) {
	if !strings.HasPrefix(stored, encryptedPrefix) {
		// Not encrypted — return as-is (backward compatibility with plaintext passwords)
		return stored, nil
	}

	if len(encryptionKey) == 0 {
		return "", fmt.Errorf("ENCRYPT_KEY not configured but password is encrypted")
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted password: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

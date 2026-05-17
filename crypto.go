package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
)

const encryptionKeyLength = 32

func getEncryptionKey() []byte {
	keyHex := os.Getenv("AMM_ENCRYPTION_KEY")
	if keyHex != "" {
		key, err := hex.DecodeString(keyHex)
		if err == nil && len(key) == encryptionKeyLength {
			return key
		}
		log.Printf("warn: invalid AMM_ENCRYPTION_KEY (need %d hex chars), trying fallback", encryptionKeyLength*2)
	}

	if db != nil {
		var storedKey string
		err := db.QueryRow("SELECT value FROM config WHERE key = ?", "encryption_key").Scan(&storedKey)
		if err == nil && storedKey != "" {
			key, err := hex.DecodeString(storedKey)
			if err == nil && len(key) == encryptionKeyLength {
				return key
			}
		}
	}

	if db != nil {
		var hash string
		err := db.QueryRow("SELECT value FROM config WHERE key = ?", adminPasswordKey).Scan(&hash)
		if err == nil && hash != "" {
			sum := sha256.Sum256([]byte("amm-enc-key:" + hash))
			return sum[:]
		}
	}

	return nil
}

func ensureEncryptionKeyExists() {
	if db == nil {
		return
	}

	var existing string
	if db.QueryRow("SELECT value FROM config WHERE key = ?", "encryption_key").Scan(&existing) == nil && existing != "" {
		return
	}

	existingKey := getEncryptionKey()
	if existingKey != nil {
		db.Exec("INSERT OR REPLACE INTO config(key, value) VALUES(?, ?)",
			"encryption_key", hex.EncodeToString(existingKey))
		return
	}

	key := make([]byte, encryptionKeyLength)
	if _, err := rand.Read(key); err != nil {
		log.Printf("warn: failed to generate encryption key: %v", err)
		return
	}
	db.Exec("INSERT OR REPLACE INTO config(key, value) VALUES(?, ?)",
		"encryption_key", hex.EncodeToString(key))
}

func encryptAPIKey(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key := getEncryptionKey()
	if key == nil {
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

func decryptAPIKey(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	key := getEncryptionKey()
	if key == nil {
		return "", fmt.Errorf("encryption key not available")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode api key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(data) < nonceSize+1 {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt api key: %w", err)
	}

	return string(plaintext), nil
}

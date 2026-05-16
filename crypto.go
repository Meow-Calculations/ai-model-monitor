package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
		var hash string
		err := db.QueryRow("SELECT value FROM config WHERE key = ?", adminPasswordKey).Scan(&hash)
		if err == nil && hash != "" {
			sum := sha256.Sum256([]byte("amm-enc-key:" + hash))
			return sum[:]
		}
	}

	return nil
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
		return encoded, nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return encoded, nil
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return encoded, nil
	}

	nonceSize := aead.NonceSize()
	if len(data) < nonceSize+1 {
		return encoded, nil
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return encoded, nil
	}

	return string(plaintext), nil
}

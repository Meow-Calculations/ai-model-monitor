package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/pbkdf2"
)

const (
	adminPasswordKey        = "admin_password_hash"
	passwordHashAlgo        = "pbkdf2_sha256"
	passwordHashIter        = 210000
	sessionTTL              = 7 * 24 * time.Hour
	loginRateLimitWindow    = 1 * time.Minute
	loginRateLimitThreshold = 5
	loginRateLimitBan       = 1 * time.Minute
)

type ipRateLimit struct {
	count       int
	windowStart time.Time
	bannedUntil time.Time
}

var (
	loginAttempts   map[string]*ipRateLimit
	loginAttemptsMu sync.Mutex
	rateLimitInit   sync.Once
)

func initRateLimiter() {
	rateLimitInit.Do(func() {
		loginAttempts = make(map[string]*ipRateLimit)
		go rateLimitCleanup()
	})
}

func rateLimitCleanup() {
	for {
		time.Sleep(5 * time.Minute)
		loginAttemptsMu.Lock()
		now := time.Now()
		for ip, entry := range loginAttempts {
			if now.Sub(entry.windowStart) > loginRateLimitWindow*2 && now.After(entry.bannedUntil) {
				delete(loginAttempts, ip)
			}
		}
		loginAttemptsMu.Unlock()
	}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip.String()
		}
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		if ip := net.ParseIP(strings.TrimSpace(real)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func checkLoginRateLimit(ip string) error {
	initRateLimiter()
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	entry, exists := loginAttempts[ip]
	now := time.Now()

	if !exists {
		entry = &ipRateLimit{}
		loginAttempts[ip] = entry
	}

	if now.Before(entry.bannedUntil) {
		remaining := entry.bannedUntil.Sub(now).Round(time.Second)
		return fmt.Errorf("登录尝试过于频繁，请在 %v 后重试", remaining)
	}

	if now.Sub(entry.windowStart) > loginRateLimitWindow {
		entry.count = 0
		entry.windowStart = now
	}

	if entry.count >= loginRateLimitThreshold {
		entry.bannedUntil = now.Add(loginRateLimitBan)
		remaining := loginRateLimitBan.Round(time.Second)
		return fmt.Errorf("登录尝试过于频繁，请在 %v 后重试", remaining)
	}

	return nil
}

func recordLoginAttempt(ip string, success bool) {
	initRateLimiter()
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	entry, exists := loginAttempts[ip]
	now := time.Now()

	if !exists {
		entry = &ipRateLimit{windowStart: now}
		loginAttempts[ip] = entry
	}

	if now.Sub(entry.windowStart) > loginRateLimitWindow {
		entry.count = 0
		entry.windowStart = now
	}

	if !success {
		entry.count++
	}
}

func IsSetupComplete() bool {
	var value string
	return db.QueryRow("SELECT value FROM config WHERE key = ?", adminPasswordKey).Scan(&value) == nil && value != ""
}

func SaveAdminPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if IsSetupComplete() {
		return fmt.Errorf("setup already completed")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO config(key, value) VALUES(?, ?)", adminPasswordKey, hash)
	return err
}

func VerifyAdminPassword(password string) bool {
	var stored string
	if err := db.QueryRow("SELECT value FROM config WHERE key = ?", adminPasswordKey).Scan(&stored); err != nil {
		return false
	}
	return verifyPassword(password, stored)
}

func CreateSession() (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	tokenHash := hashSessionToken(token)
	expiresAt := time.Now().Add(sessionTTL).Format("2006-01-02 15:04:05")
	_, err = db.Exec("INSERT INTO sessions(token_hash, expires_at) VALUES(?, ?)", tokenHash, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

func ValidateSession(token string) bool {
	if token == "" {
		return false
	}
	var expiresAt string
	if err := db.QueryRow("SELECT expires_at FROM sessions WHERE token_hash = ?", hashSessionToken(token)).Scan(&expiresAt); err != nil {
		return false
	}
	expires, err := time.Parse("2006-01-02 15:04:05", expiresAt)
	if err != nil || time.Now().After(expires) {
		ClearSession(token)
		return false
	}
	return true
}

func ClearSession(token string) {
	if token == "" {
		return
	}
	db.Exec("DELETE FROM sessions WHERE token_hash = ?", hashSessionToken(token))
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIter, 32)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		passwordHashAlgo,
		strconv.Itoa(passwordHashIter),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	}, "$"), nil
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != passwordHashAlgo {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iter, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

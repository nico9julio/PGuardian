package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid PIN or master password")
	ErrRateLimited        = errors.New("too many failed attempts, please wait before trying again")
	ErrUnauthorized       = errors.New("session expired or unauthorized")
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // 64 MB during brief verification
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	sessionTTL          = 12 * time.Hour
	maxSessions         = 32
)

// HashPassword generates an Argon2id encoded hash string.
func HashPassword(password string) (string, error) {
	if len(strings.TrimSpace(password)) < 4 {
		return "", errors.New("master password or PIN must be at least 4 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword compares a candidate password/PIN against an Argon2id encoded hash in constant time.
func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported password hash format")
	}
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, t, mem, p, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1, nil
}

type sessionEntry struct {
	expiresAt time.Time
}

// Manager handles session tokens and brute-force protection with bounded memory.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]sessionEntry
	failedCount  int
	lockedUntil  time.Time
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]sessionEntry),
	}
}

func (m *Manager) Authenticate(password, storedHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if now.Before(m.lockedUntil) {
		return "", ErrRateLimited
	}

	ok, err := VerifyPassword(password, storedHash)
	if err != nil || !ok {
		m.failedCount++
		if m.failedCount >= 5 {
			m.lockedUntil = now.Add(30 * time.Second)
			m.failedCount = 0
		}
		return "", ErrInvalidCredentials
	}

	m.failedCount = 0
	m.pruneExpiredLocked(now)

	// Cap active sessions to prevent any unbounded map growth
	if len(m.sessions) >= maxSessions {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range m.sessions {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		if oldestKey != "" {
			delete(m.sessions, oldestKey)
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	m.sessions[token] = sessionEntry{expiresAt: now.Add(sessionTTL)}
	return token, nil
}

// IssueInitialToken issues an authenticated session right after First-Run Wizard completes.
func (m *Manager) IssueInitialToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	m.sessions[token] = sessionEntry{expiresAt: time.Now().Add(sessionTTL)}
	return token, nil
}

func (m *Manager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	entry, exists := m.sessions[token]
	if !exists {
		return false
	}
	if now.After(entry.expiresAt) {
		delete(m.sessions, token)
		return false
	}
	// Sliding expiration
	entry.expiresAt = now.Add(sessionTTL)
	m.sessions[token] = entry
	return true
}

func (m *Manager) RevokeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]sessionEntry)
}

func (m *Manager) pruneExpiredLocked(now time.Time) {
	for k, v := range m.sessions {
		if now.After(v.expiresAt) {
			delete(m.sessions, k)
		}
	}
}

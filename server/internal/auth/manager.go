package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultPassword  = "admin"
	passwordRounds   = 210_000
	passwordKeyBytes = 32
	sessionLifetime  = 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTemporarilyLocked  = errors.New("authentication temporarily locked")
	ErrPasswordTooShort   = errors.New("new password must contain at least 8 characters")
	ErrPasswordTooLong    = errors.New("new password must contain at most 128 characters")
	ErrPasswordUnchanged  = errors.New("new password must differ from current password")
)

type credentialFile struct {
	Version         int    `json:"version"`
	Salt            string `json:"salt"`
	Hash            string `json:"hash"`
	Iterations      int    `json:"iterations"`
	PasswordChanged bool   `json:"passwordChanged"`
}

type Manager struct {
	mu             sync.Mutex
	path           string
	credentials    credentialFile
	sessions       map[string]time.Time
	failedAttempts int
	lockedUntil    time.Time
	now            func() time.Time
}

func Open(path string) (*Manager, error) {
	manager := &Manager{path: path, sessions: make(map[string]time.Time), now: time.Now}
	encoded, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(encoded) == 0 {
		if err := manager.setPasswordLocked(DefaultPassword, false); err != nil {
			return nil, err
		}
		if err := manager.persistLocked(); err != nil {
			return nil, err
		}
		return manager, nil
	}
	if err := json.Unmarshal(encoded, &manager.credentials); err != nil {
		return nil, fmt.Errorf("decode credentials: %w", err)
	}
	if manager.credentials.Version != 1 || manager.credentials.Iterations < 100_000 {
		return nil, errors.New("unsupported credentials format")
	}
	if _, err := base64.RawStdEncoding.DecodeString(manager.credentials.Salt); err != nil {
		return nil, fmt.Errorf("decode credential salt: %w", err)
	}
	if _, err := base64.RawStdEncoding.DecodeString(manager.credentials.Hash); err != nil {
		return nil, fmt.Errorf("decode credential hash: %w", err)
	}
	return manager, nil
}

func (m *Manager) Login(password string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if now.Before(m.lockedUntil) {
		return "", m.mustChangePasswordLocked(), ErrTemporarilyLocked
	}
	if !m.verifyPasswordLocked(password) {
		m.failedAttempts++
		if m.failedAttempts >= 5 {
			m.failedAttempts = 0
			m.lockedUntil = now.Add(30 * time.Second)
		}
		return "", m.mustChangePasswordLocked(), ErrInvalidCredentials
	}
	m.failedAttempts = 0
	m.lockedUntil = time.Time{}
	token, err := randomToken(32)
	if err != nil {
		return "", m.mustChangePasswordLocked(), err
	}
	m.cleanupSessionsLocked(now)
	m.sessions[token] = now.Add(sessionLifetime)
	return token, m.mustChangePasswordLocked(), nil
}

func (m *Manager) ValidateSession(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	expiresAt, ok := m.sessions[token]
	if !ok || !m.now().Before(expiresAt) {
		delete(m.sessions, token)
		return false
	}
	return true
}

func (m *Manager) Logout(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

func (m *Manager) MustChangePassword() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mustChangePasswordLocked()
}

func (m *Manager) ChangePassword(currentPassword, newPassword string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.verifyPasswordLocked(currentPassword) {
		return "", ErrInvalidCredentials
	}
	length := utf8.RuneCountInString(newPassword)
	if length < 8 {
		return "", ErrPasswordTooShort
	}
	if length > 128 {
		return "", ErrPasswordTooLong
	}
	if currentPassword == newPassword {
		return "", ErrPasswordUnchanged
	}
	if err := m.setPasswordLocked(newPassword, true); err != nil {
		return "", err
	}
	if err := m.persistLocked(); err != nil {
		return "", err
	}
	clear(m.sessions)
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	m.sessions[token] = m.now().Add(sessionLifetime)
	return token, nil
}

func (m *Manager) mustChangePasswordLocked() bool {
	return !m.credentials.PasswordChanged
}

func (m *Manager) verifyPasswordLocked(password string) bool {
	salt, err := base64.RawStdEncoding.DecodeString(m.credentials.Salt)
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(m.credentials.Hash)
	if err != nil {
		return false
	}
	actual := deriveKey([]byte(password), salt, m.credentials.Iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (m *Manager) setPasswordLocked(password string, changed bool) error {
	salt := make([]byte, 24)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := deriveKey([]byte(password), salt, passwordRounds, passwordKeyBytes)
	m.credentials = credentialFile{
		Version:         1,
		Salt:            base64.RawStdEncoding.EncodeToString(salt),
		Hash:            base64.RawStdEncoding.EncodeToString(hash),
		Iterations:      passwordRounds,
		PasswordChanged: changed,
	}
	return nil
}

func (m *Manager) persistLocked() error {
	encoded, err := json.MarshalIndent(m.credentials, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	temporary := m.path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, m.path)
}

func (m *Manager) cleanupSessionsLocked(now time.Time) {
	for token, expiresAt := range m.sessions {
		if !now.Before(expiresAt) {
			delete(m.sessions, token)
		}
	}
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func deriveKey(password, salt []byte, iterations, keyLength int) []byte {
	result := make([]byte, 0, keyLength)
	for block := uint32(1); len(result) < keyLength; block++ {
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		counter := make([]byte, 4)
		binary.BigEndian.PutUint32(counter, block)
		_, _ = mac.Write(counter)
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for round := 1; round < iterations; round++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for index := range t {
				t[index] ^= u[index]
			}
		}
		result = append(result, t...)
	}
	return result[:keyLength]
}

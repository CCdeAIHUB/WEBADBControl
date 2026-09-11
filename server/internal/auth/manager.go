package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/coreipc"
)

const (
	DefaultAdminUsername = "admin"
	DefaultPassword      = "admin"
	sessionLifetime      = 8 * time.Hour
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

var (
	ErrAdminOnly          = errors.New("only the built-in Core administrator may open a Web session")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTemporarilyLocked  = errors.New("authentication temporarily locked")
	ErrPasswordUnchanged  = errors.New("new password must differ from current password")
)

type Account struct {
	Username               string   `json:"username"`
	Role                   Role     `json:"role"`
	Devices                []string `json:"devices"`
	BuiltIn                bool     `json:"builtIn"`
	PasswordChangeRequired bool     `json:"passwordChangeRequired"`
}

type Session struct {
	Username               string `json:"username"`
	Role                   Role   `json:"role"`
	PasswordChangeRequired bool   `json:"passwordChangeRequired"`
	LocalBypass            bool   `json:"localBypass,omitempty"`
	coreSessionToken       string
}

type browserSession struct {
	session   Session
	expiresAt time.Time
}

type Manager struct {
	mu             sync.Mutex
	core           coreipc.Caller
	sessions       map[string]browserSession
	now            func() time.Time
	failedAttempts int
	lockedUntil    time.Time
	localBootstrap *Session
}

func New(core coreipc.Caller) *Manager {
	return &Manager{core: core, sessions: map[string]browserSession{}, now: time.Now}
}

func (m *Manager) Login(ctx context.Context, password string) (string, Session, error) {
	m.mu.Lock()
	if m.now().Before(m.lockedUntil) {
		m.mu.Unlock()
		return "", Session{}, ErrTemporarilyLocked
	}
	m.mu.Unlock()
	session, err := m.loginCore(ctx, password)
	if err != nil {
		if coreErrorCode(err) == "REMOTE_AUTH_CREDENTIALS_INVALID" {
			m.mu.Lock()
			m.failedAttempts++
			if m.failedAttempts >= 5 {
				m.failedAttempts = 0
				m.lockedUntil = m.now().Add(30 * time.Second)
			}
			m.mu.Unlock()
			return "", Session{}, fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
		}
		return "", Session{}, err
	}
	if session.Username != DefaultAdminUsername || session.Role != RoleAdmin || session.coreSessionToken == "" {
		return "", Session{}, ErrAdminOnly
	}
	token, err := randomToken(32)
	if err != nil {
		return "", Session{}, err
	}
	m.mu.Lock()
	m.failedAttempts = 0
	m.lockedUntil = time.Time{}
	m.cleanupLocked(m.now())
	m.sessions[token] = browserSession{session: session, expiresAt: m.now().Add(sessionLifetime)}
	m.mu.Unlock()
	return token, session, nil
}

func (m *Manager) Authenticate(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(m.now())
	record, ok := m.sessions[token]
	return record.session, ok
}

// LocalBootstrapSession grants local management access only while Core still
// accepts its factory administrator password. Core remains the only password
// authority; this adapter stores no credential.
func (m *Manager) LocalBootstrapSession(ctx context.Context) (Session, bool) {
	m.mu.Lock()
	var cached Session
	if m.localBootstrap != nil {
		cached = *m.localBootstrap
	}
	m.mu.Unlock()

	if cached.coreSessionToken != "" {
		if refreshed, ok := m.refreshCoreSession(ctx, cached.coreSessionToken); ok && refreshed.PasswordChangeRequired {
			refreshed.LocalBypass = true
			m.mu.Lock()
			m.localBootstrap = &refreshed
			m.mu.Unlock()
			return refreshed, true
		}
		m.mu.Lock()
		m.localBootstrap = nil
		m.mu.Unlock()
	}

	session, err := m.loginCore(ctx, DefaultPassword)
	if err != nil || session.Username != DefaultAdminUsername || session.Role != RoleAdmin || !session.PasswordChangeRequired {
		return Session{}, false
	}
	session.LocalBypass = true
	m.mu.Lock()
	m.localBootstrap = &session
	m.mu.Unlock()
	return session, true
}

func (m *Manager) DefaultPasswordActive(ctx context.Context) bool {
	_, active := m.LocalBootstrapSession(ctx)
	return active
}

func (m *Manager) refreshCoreSession(ctx context.Context, token string) (Session, bool) {
	var result struct {
		Username               string `json:"username"`
		Role                   Role   `json:"role"`
		PasswordChangeRequired bool   `json:"passwordChangeRequired"`
	}
	if err := m.core.Call(ctx, "remote.admin.me", map[string]any{"sessionToken": token}, &result); err != nil {
		return Session{}, false
	}
	if result.Username != DefaultAdminUsername || result.Role != RoleAdmin {
		return Session{}, false
	}
	return Session{Username: result.Username, Role: result.Role, PasswordChangeRequired: result.PasswordChangeRequired, coreSessionToken: token}, true
}

func (m *Manager) Logout(ctx context.Context, token string) error {
	m.mu.Lock()
	record, ok := m.sessions[token]
	delete(m.sessions, token)
	m.mu.Unlock()
	if ok {
		var ignored map[string]any
		return m.core.Call(ctx, "remote.admin.logout", map[string]any{"sessionToken": record.session.coreSessionToken}, &ignored)
	}
	return nil
}

func (m *Manager) ChangeAdminPassword(ctx context.Context, session Session, currentPassword, newPassword string) (string, Session, error) {
	if currentPassword == newPassword {
		return "", Session{}, ErrPasswordUnchanged
	}
	if err := requireAdmin(session); err != nil {
		return "", Session{}, err
	}
	var ignored map[string]any
	err := m.core.Call(ctx, "remote.admin.changePassword", map[string]any{
		"sessionToken":    session.coreSessionToken,
		"currentPassword": currentPassword,
		"newPassword":     newPassword,
	}, &ignored)
	if err != nil {
		if coreErrorCode(err) == "REMOTE_AUTH_CREDENTIALS_INVALID" {
			return "", Session{}, fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
		}
		return "", Session{}, err
	}
	m.mu.Lock()
	m.sessions = map[string]browserSession{}
	m.localBootstrap = nil
	m.mu.Unlock()
	return m.Login(ctx, newPassword)
}

func (m *Manager) ListRemoteUsers(ctx context.Context, session Session) ([]Account, error) {
	if err := requireAdmin(session); err != nil {
		return nil, err
	}
	var accounts []Account
	err := m.core.Call(ctx, "remote.admin.users.list", adminParams(session, nil), &accounts)
	if err != nil {
		return nil, err
	}
	users := accounts[:0]
	for _, account := range accounts {
		if account.Role == RoleUser && account.Username != DefaultAdminUsername {
			users = append(users, account)
		}
	}
	return users, nil
}

func (m *Manager) CreateRemoteUser(ctx context.Context, session Session, username, password string) (Account, error) {
	if err := requireAdmin(session); err != nil {
		return Account{}, err
	}
	var account Account
	err := m.core.Call(ctx, "remote.admin.users.create", adminParams(session, map[string]any{"username": username, "password": password}), &account)
	return account, err
}

func (m *Manager) DeleteRemoteUser(ctx context.Context, session Session, username string) error {
	if err := requireAdmin(session); err != nil {
		return err
	}
	var ignored map[string]any
	return m.core.Call(ctx, "remote.admin.users.delete", adminParams(session, map[string]any{"username": username}), &ignored)
}

func (m *Manager) ResetRemoteUserPassword(ctx context.Context, session Session, username, newPassword string) error {
	if err := requireAdmin(session); err != nil {
		return err
	}
	var ignored map[string]any
	return m.core.Call(ctx, "remote.admin.users.resetPassword", adminParams(session, map[string]any{"username": username, "newPassword": newPassword}), &ignored)
}

func (m *Manager) SetRemoteUserDevice(ctx context.Context, session Session, username, deviceID string, assigned bool) (Account, error) {
	if err := requireAdmin(session); err != nil {
		return Account{}, err
	}
	var account Account
	err := m.core.Call(ctx, "remote.admin.devices.assign", adminParams(session, map[string]any{"username": username, "deviceId": deviceID, "assigned": assigned}), &account)
	return account, err
}

func (m *Manager) loginCore(ctx context.Context, password string) (Session, error) {
	var result struct {
		SessionToken           string `json:"sessionToken"`
		Username               string `json:"username"`
		Role                   Role   `json:"role"`
		PasswordChangeRequired bool   `json:"passwordChangeRequired"`
	}
	if err := m.core.Call(ctx, "remote.admin.login", map[string]any{"password": password}, &result); err != nil {
		return Session{}, err
	}
	return Session{
		Username:               result.Username,
		Role:                   result.Role,
		PasswordChangeRequired: result.PasswordChangeRequired,
		coreSessionToken:       result.SessionToken,
	}, nil
}

func requireAdmin(session Session) error {
	if session.Username != DefaultAdminUsername || session.Role != RoleAdmin || session.coreSessionToken == "" {
		return ErrAdminOnly
	}
	return nil
}

func adminParams(session Session, values map[string]any) map[string]any {
	params := map[string]any{"sessionToken": session.coreSessionToken}
	for key, value := range values {
		params[key] = value
	}
	return params
}

func (m *Manager) cleanupLocked(now time.Time) {
	for token, session := range m.sessions {
		if !now.Before(session.expiresAt) {
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

func coreErrorCode(err error) string {
	var appError *apperror.Error
	if errors.As(err, &appError) {
		return appError.ErrorCode
	}
	return ""
}

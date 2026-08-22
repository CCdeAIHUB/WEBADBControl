package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPasswordAndChangeFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	manager, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials must be persisted with mode 0600: info=%v err=%v", info, err)
	}
	token, mustChange, err := manager.Login(DefaultPassword)
	if err != nil || token == "" || !mustChange || !manager.ValidateSession(token) {
		t.Fatalf("default login failed: token=%q mustChange=%v err=%v", token, mustChange, err)
	}
	newToken, err := manager.ChangePassword(DefaultPassword, "new-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if manager.ValidateSession(token) || !manager.ValidateSession(newToken) || manager.MustChangePassword() {
		t.Fatal("password change must rotate sessions and clear bootstrap state")
	}
	if _, _, err := manager.Login(DefaultPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password must be rejected, got %v", err)
	}
	if _, mustChange, err := manager.Login("new-password-123"); err != nil || mustChange {
		t.Fatalf("new password login failed: mustChange=%v err=%v", mustChange, err)
	}
}

func TestPasswordPolicy(t *testing.T) {
	manager, err := Open(filepath.Join(t.TempDir(), "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ChangePassword(DefaultPassword, "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected short-password error, got %v", err)
	}
	if _, err := manager.ChangePassword("wrong", "new-password-123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid-credentials error, got %v", err)
	}
}

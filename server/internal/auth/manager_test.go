package auth

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type recordedCall struct {
	method string
	params map[string]any
}

type fakeCore struct {
	calls    []recordedCall
	password string
	role     Role
}

func (f *fakeCore) Call(_ context.Context, method string, params any, target any) error {
	encoded, _ := json.Marshal(params)
	var values map[string]any
	_ = json.Unmarshal(encoded, &values)
	f.calls = append(f.calls, recordedCall{method: method, params: values})

	switch method {
	case "remote.admin.login":
		if values["password"] != f.password {
			return apperror.New("REMOTE_AUTH_CREDENTIALS_INVALID", "invalid", "remote.auth", true)
		}
		return assignResult(target, map[string]any{
			"sessionToken":           "core-admin-token",
			"username":               DefaultAdminUsername,
			"role":                   f.role,
			"passwordChangeRequired": f.password == DefaultPassword,
		})
	case "remote.admin.changePassword":
		if values["currentPassword"] != f.password {
			return apperror.New("REMOTE_AUTH_CREDENTIALS_INVALID", "invalid", "remote.auth", true)
		}
		f.password = values["newPassword"].(string)
		return assignResult(target, map[string]any{"passwordChanged": true, "loginRequired": true})
	case "remote.admin.me":
		return assignResult(target, map[string]any{"username": DefaultAdminUsername, "role": f.role, "passwordChangeRequired": f.password == DefaultPassword})
	case "remote.admin.users.list":
		return assignResult(target, []Account{{Username: "operator", Role: RoleUser}})
	default:
		return assignResult(target, map[string]any{"ok": true})
	}
}

func TestLocalBootstrapExistsOnlyWhileCoreDefaultPasswordIsActive(t *testing.T) {
	core := &fakeCore{password: DefaultPassword, role: RoleAdmin}
	manager := New(core)
	session, ok := manager.LocalBootstrapSession(context.Background())
	if !ok || !session.LocalBypass || !session.PasswordChangeRequired {
		t.Fatalf("default Core password should allow local bootstrap: %+v ok=%v", session, ok)
	}
	if _, _, err := manager.ChangeAdminPassword(context.Background(), session, DefaultPassword, "secure-admin-password"); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.LocalBootstrapSession(context.Background()); ok {
		t.Fatal("local bootstrap must stop after the Core administrator password changes")
	}
}

func assignResult(target any, value any) error {
	if target == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return json.Unmarshal(encoded, target)
}

func TestWebLoginUsesOnlyCoreBuiltinAdministrator(t *testing.T) {
	// Scenario: Web login authenticates against Core's built-in administrator;
	// a non-admin Core response must never create a browser session.
	core := &fakeCore{password: DefaultPassword, role: RoleAdmin}
	manager := New(core)

	token, session, err := manager.Login(context.Background(), DefaultPassword)
	if err != nil || token == "" || session.Username != DefaultAdminUsername || !session.PasswordChangeRequired {
		t.Fatalf("login: token=%q session=%+v err=%v", token, session, err)
	}
	if core.calls[0].method != "remote.admin.login" || core.calls[0].params["password"] != DefaultPassword {
		t.Fatalf("unexpected Core call: %+v", core.calls[0])
	}

	core.role = RoleUser
	if _, _, err := New(core).Login(context.Background(), DefaultPassword); !errors.Is(err, ErrAdminOnly) {
		t.Fatalf("non-admin Core identity must be rejected, got %v", err)
	}
}

func TestAdminPasswordChangeRotatesCoreAndBrowserSessions(t *testing.T) {
	// Scenario: changing the Web password changes Core's administrator password,
	// revokes the old browser cookie, then creates a fresh administrator session.
	core := &fakeCore{password: DefaultPassword, role: RoleAdmin}
	manager := New(core)
	oldToken, session, err := manager.Login(context.Background(), DefaultPassword)
	if err != nil {
		t.Fatal(err)
	}

	newToken, changed, err := manager.ChangeAdminPassword(
		context.Background(),
		session,
		DefaultPassword,
		"secure-admin-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Authenticate(oldToken); ok {
		t.Fatal("old browser session must be revoked")
	}
	if authenticated, ok := manager.Authenticate(newToken); !ok || authenticated.PasswordChangeRequired || changed.PasswordChangeRequired {
		t.Fatalf("new session invalid: authenticated=%+v changed=%+v", authenticated, changed)
	}
	if core.password != "secure-admin-password" {
		t.Fatal("Core administrator password was not changed")
	}
}

func TestRemoteUserManagementCarriesCoreAdminToken(t *testing.T) {
	// Scenario: ordinary users are managed in Core; Go must not persist a second
	// account database and must keep the Core token server-side.
	core := &fakeCore{password: "secure-admin-password", role: RoleAdmin}
	manager := New(core)
	_, session, err := manager.Login(context.Background(), core.password)
	if err != nil {
		t.Fatal(err)
	}

	users, err := manager.ListRemoteUsers(context.Background(), session)
	if err != nil || !reflect.DeepEqual(users, []Account{{Username: "operator", Role: RoleUser}}) {
		t.Fatalf("users=%+v err=%v", users, err)
	}
	last := core.calls[len(core.calls)-1]
	if last.method != "remote.admin.users.list" || last.params["sessionToken"] != "core-admin-token" {
		t.Fatalf("unexpected management call: %+v", last)
	}
}

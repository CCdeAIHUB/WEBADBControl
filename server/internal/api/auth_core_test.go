package api

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
)

type testAuthCore struct {
	mu       sync.Mutex
	password string
	role     auth.Role
	users    map[string]auth.Account
}

func newTestAuthManager() (*auth.Manager, *testAuthCore) {
	core := &testAuthCore{
		password: auth.DefaultPassword,
		role:     auth.RoleAdmin,
		users:    make(map[string]auth.Account),
	}
	return auth.New(core), core
}

func (c *testAuthCore) Call(_ context.Context, method string, params any, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	values := testParams(params)
	switch method {
	case "remote.admin.login":
		if values["password"] != c.password {
			return apperror.New("REMOTE_AUTH_CREDENTIALS_INVALID", "invalid credentials", "remote.auth", false)
		}
		return testAssign(target, map[string]any{
			"sessionToken":           "core-admin-session",
			"username":               auth.DefaultAdminUsername,
			"role":                   c.role,
			"passwordChangeRequired": c.password == auth.DefaultPassword,
		})
	case "remote.admin.logout":
		return nil
	case "remote.admin.me":
		return testAssign(target, map[string]any{"username": auth.DefaultAdminUsername, "role": c.role, "passwordChangeRequired": c.password == auth.DefaultPassword})
	case "remote.admin.changePassword":
		if values["currentPassword"] != c.password {
			return apperror.New("REMOTE_AUTH_CREDENTIALS_INVALID", "invalid credentials", "remote.auth", false)
		}
		c.password, _ = values["newPassword"].(string)
		return testAssign(target, map[string]bool{"passwordChanged": true})
	case "remote.admin.users.list":
		users := make([]auth.Account, 0, len(c.users))
		for _, account := range c.users {
			users = append(users, account)
		}
		return testAssign(target, users)
	case "remote.admin.users.create":
		username, _ := values["username"].(string)
		if _, exists := c.users[username]; exists {
			return apperror.New("REMOTE_AUTH_USER_EXISTS", "user exists", "remote.auth", false)
		}
		account := auth.Account{Username: username, Role: auth.RoleUser, Devices: []string{}}
		c.users[username] = account
		return testAssign(target, account)
	case "remote.admin.users.delete":
		username, _ := values["username"].(string)
		if _, exists := c.users[username]; !exists {
			return apperror.New("REMOTE_AUTH_USER_NOT_FOUND", "user not found", "remote.auth", false)
		}
		delete(c.users, username)
		return nil
	case "remote.admin.users.resetPassword":
		return nil
	case "remote.admin.devices.assign":
		username, _ := values["username"].(string)
		account, exists := c.users[username]
		if !exists {
			return apperror.New("REMOTE_AUTH_USER_NOT_FOUND", "user not found", "remote.auth", false)
		}
		deviceID, _ := values["deviceId"].(string)
		assigned, _ := values["assigned"].(bool)
		account.Devices = updateTestDevices(account.Devices, deviceID, assigned)
		c.users[username] = account
		return testAssign(target, account)
	default:
		return apperror.New("IPC_METHOD_NOT_FOUND", "unknown test method", "ipc.protocol", false)
	}
}

func testParams(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	return result
}

func testAssign(target any, value any) error {
	if target == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return json.Unmarshal(encoded, target)
}

func updateTestDevices(devices []string, deviceID string, assigned bool) []string {
	result := make([]string, 0, len(devices)+1)
	for _, existing := range devices {
		if existing != deviceID {
			result = append(result, existing)
		}
	}
	if assigned {
		result = append(result, deviceID)
	}
	return result
}

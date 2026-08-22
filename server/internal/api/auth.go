package api

import "github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"

func errUnauthorized() error {
	return apperror.New("AUTH_REQUIRED", "登录状态已失效，请重新登录", "api.auth", true)
}

func errPasswordChangeRequired() error {
	return apperror.New("AUTH_PASSWORD_CHANGE_REQUIRED", "首次登录必须先修改默认密码", "api.auth", true)
}

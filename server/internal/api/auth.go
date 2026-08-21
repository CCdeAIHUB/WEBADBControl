package api

import "github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"

func errUnauthorized() error {
	return apperror.New("AUTH_REQUIRED", "需要有效的访问令牌", "api.auth", true)
}

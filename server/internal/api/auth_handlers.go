package api

import (
	"errors"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
)

const sessionCookieName = "webadb_session"

type sessionResponse struct {
	Authenticated      bool      `json:"authenticated"`
	MustChangePassword bool      `json:"mustChangePassword"`
	Username           string    `json:"username,omitempty"`
	Role               auth.Role `json:"role,omitempty"`
	LocalBypass        bool      `json:"localBypass,omitempty"`
}

func (s *Server) sessionStatus(writer http.ResponseWriter, request *http.Request) {
	var session auth.Session
	authenticated := false
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		session, authenticated = s.auth.Authenticate(cookie.Value)
	}
	if !authenticated && isLocalManagementRequest(s.config.Address, request) {
		session, authenticated = s.auth.LocalBootstrapSession(request.Context())
	}
	writeData(writer, http.StatusOK, sessionResponse{
		Authenticated:      authenticated,
		MustChangePassword: authenticated && session.PasswordChangeRequired,
		Username:           session.Username,
		Role:               session.Role,
		LocalBypass:        session.LocalBypass,
	})
}

func (s *Server) createSession(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		// Username is accepted only for a clear admin-only error from older clients.
		Username string `json:"username,omitempty"`
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.Username != "" && body.Username != auth.DefaultAdminUsername {
		writeError(writer, http.StatusForbidden, apperror.New(
			"AUTH_ADMIN_ONLY",
			"Web 控制台仅允许内置管理员登录；远程用户请使用远程控制客户端",
			"api.auth",
			false,
		))
		return
	}
	token, session, err := s.auth.Login(request.Context(), body.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTemporarilyLocked):
			writeError(writer, http.StatusTooManyRequests, apperror.New("AUTH_RATE_LIMITED", "登录失败次数过多，请 30 秒后重试", "api.auth", true))
		case errors.Is(err, auth.ErrAdminOnly):
			writeError(writer, http.StatusForbidden, apperror.New("AUTH_ADMIN_ONLY", "Web 控制台仅允许内置管理员登录", "api.auth", false))
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(writer, http.StatusUnauthorized, apperror.New("AUTH_INVALID_PASSWORD", "管理员密码不正确", "api.auth", true))
		default:
			writeError(writer, http.StatusBadGateway, err)
		}
		return
	}
	s.setSessionCookie(writer, token)
	writeData(writer, http.StatusOK, responseFromSession(session))
}

func (s *Server) deleteSession(writer http.ResponseWriter, request *http.Request) {
	var logoutErr error
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		logoutErr = s.auth.Logout(request.Context(), cookie.Value)
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.config.BehindProxy,
		MaxAge:   -1,
	})
	if logoutErr != nil {
		writeError(writer, http.StatusBadGateway, logoutErr)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) changePassword(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	token, changed, err := s.auth.ChangeAdminPassword(
		request.Context(),
		session,
		body.CurrentPassword,
		body.NewPassword,
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_CURRENT_PASSWORD_INVALID", "当前管理员密码不正确", "api.auth", true))
		case errors.Is(err, auth.ErrPasswordUnchanged):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_PASSWORD_UNCHANGED", "新密码不能与当前密码相同", "api.auth", true))
		case appErrorCode(err) == "REMOTE_AUTH_PASSWORD_INVALID":
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_PASSWORD_INVALID", "新密码必须为 8–1024 个字节", "api.auth", true))
		case errors.Is(err, auth.ErrAdminOnly):
			writeError(writer, http.StatusForbidden, apperror.New("AUTH_ADMIN_ONLY", "仅内置管理员可修改控制台密码", "api.auth", false))
		default:
			writeError(writer, http.StatusBadGateway, err)
		}
		return
	}
	s.setSessionCookie(writer, token)
	writeData(writer, http.StatusOK, responseFromSession(changed))
}

func responseFromSession(session auth.Session) sessionResponse {
	return sessionResponse{
		Authenticated:      true,
		MustChangePassword: session.PasswordChangeRequired,
		Username:           session.Username,
		Role:               session.Role,
		LocalBypass:        session.LocalBypass,
	}
}

func appErrorCode(err error) string {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.ErrorCode
	}
	return ""
}

func (s *Server) setSessionCookie(writer http.ResponseWriter, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.config.BehindProxy,
		MaxAge:   8 * 60 * 60,
	})
}

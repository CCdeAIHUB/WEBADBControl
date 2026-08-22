package api

import (
	"errors"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
)

const sessionCookieName = "webadb_session"

type sessionResponse struct {
	Authenticated      bool `json:"authenticated"`
	MustChangePassword bool `json:"mustChangePassword"`
}

func (s *Server) sessionStatus(writer http.ResponseWriter, request *http.Request) {
	authenticated := false
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		authenticated = s.auth.ValidateSession(cookie.Value)
	}
	writeData(writer, http.StatusOK, sessionResponse{Authenticated: authenticated, MustChangePassword: authenticated && s.auth.MustChangePassword()})
}

func (s *Server) createSession(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	token, mustChange, err := s.auth.Login(body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrTemporarilyLocked) {
			writeError(writer, http.StatusTooManyRequests, apperror.New("AUTH_RATE_LIMITED", "登录失败次数过多，请 30 秒后重试", "api.auth", true))
			return
		}
		writeError(writer, http.StatusUnauthorized, apperror.New("AUTH_INVALID_PASSWORD", "密码不正确", "api.auth", true))
		return
	}
	s.setSessionCookie(writer, token)
	writeData(writer, http.StatusOK, sessionResponse{Authenticated: true, MustChangePassword: mustChange})
}

func (s *Server) deleteSession(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		s.auth.Logout(cookie.Value)
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.config.BehindProxy, MaxAge: -1})
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
	token, err := s.auth.ChangePassword(body.CurrentPassword, body.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_CURRENT_PASSWORD_INVALID", "当前密码不正确", "api.auth", true))
		case errors.Is(err, auth.ErrPasswordTooShort):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_PASSWORD_TOO_SHORT", "新密码至少需要 8 个字符", "api.auth", true))
		case errors.Is(err, auth.ErrPasswordTooLong):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_PASSWORD_TOO_LONG", "新密码不能超过 128 个字符", "api.auth", true))
		case errors.Is(err, auth.ErrPasswordUnchanged):
			writeError(writer, http.StatusBadRequest, apperror.New("AUTH_PASSWORD_UNCHANGED", "新密码不能与当前密码相同", "api.auth", true))
		default:
			writeError(writer, http.StatusInternalServerError, err)
		}
		return
	}
	s.setSessionCookie(writer, token)
	writeData(writer, http.StatusOK, sessionResponse{Authenticated: true, MustChangePassword: false})
}

func (s *Server) setSessionCookie(writer http.ResponseWriter, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.config.BehindProxy,
		MaxAge:   86400,
	})
}

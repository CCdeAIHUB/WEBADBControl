package api

import (
	"context"
	"net"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
)

type remoteSession struct {
	Username               string   `json:"username"`
	Role                   string   `json:"role"`
	Devices                []string `json:"devices"`
	PasswordChangeRequired bool     `json:"passwordChangeRequired"`
}

type remoteSessionContextKey struct{}

func withRemoteSession(request *http.Request, session remoteSession) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), remoteSessionContextKey{}, session))
}

func remoteSessionFromContext(ctx context.Context) (remoteSession, bool) {
	session, ok := ctx.Value(remoteSessionContextKey{}).(remoteSession)
	return session, ok
}

func (s remoteSession) canAccessDevice(deviceID string) bool {
	for _, assigned := range s.Devices {
		if assigned == deviceID {
			return true
		}
	}
	return false
}

func isLocalManagementRequest(listenAddress string, request *http.Request) bool {
	peerHost, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		peerHost = request.RemoteAddr
	}
	if peerIP := net.ParseIP(peerHost); peerIP != nil && peerIP.IsLoopback() {
		return true
	}
	listenHost, _, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return false
	}
	listenIP := net.ParseIP(listenHost)
	return listenIP != nil && listenIP.IsLoopback()
}

type sessionContextKey struct{}

func withSession(request *http.Request, session auth.Session) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), sessionContextKey{}, session))
}

func sessionFromContext(ctx context.Context) (auth.Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(auth.Session)
	return session, ok
}

package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
)

func TestStaticIndexIsNeverCachedAcrossDeployments(t *testing.T) {
	// 场景：发布新前端后，浏览器不能继续使用引用旧动态分块的 index.html。
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<div>current release</div>"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{config: config.Config{WebDir: webDir}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	response := httptest.NewRecorder()
	server.static(response, httptest.NewRequest(http.MethodGet, "/devices/phone", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "current release") {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}
}

func TestStaticMissingAssetReturns404InsteadOfSPAHTML(t *testing.T) {
	// 场景：旧页面请求已被新版本移除的解码分块时必须返回 404，不能伪装成 JavaScript 的 index.html。
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<div>spa</div>"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{config: config.Config{WebDir: webDir}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	response := httptest.NewRecorder()
	server.static(response, httptest.NewRequest(http.MethodGet, "/assets/removed-decoder.js", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q, want 404", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "<div>spa</div>") {
		t.Fatal("missing asset must not fall back to SPA HTML")
	}
}

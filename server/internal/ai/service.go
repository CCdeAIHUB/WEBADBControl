package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/settings"
)

type Request struct {
	ModelID     string            `json:"modelId"`
	Messages    []json.RawMessage `json:"messages"`
	Tools       []json.RawMessage `json:"tools,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
}

type Service struct {
	settings *settings.Store
	client   *http.Client
}

func NewService(store *settings.Store) *Service {
	return &Service{settings: store, client: &http.Client{Timeout: 5 * time.Minute}}
}

func (s *Service) Stream(ctx context.Context, request Request) (*http.Response, error) {
	model, ok := s.settings.Model(request.ModelID)
	if !ok {
		return nil, apperror.New("AI_MODEL_NOT_FOUND", "未找到可用的 AI 模型配置", "ai.service", true)
	}
	encodedMessages, _ := json.Marshal(request.Messages)
	if !model.Vision && bytes.Contains(encodedMessages, []byte(`"image_url"`)) {
		return nil, apperror.New("AI_VISION_UNSUPPORTED", "当前模型不支持屏幕图像", "ai.compatibility", true).
			WithSuggestion("请选择支持视觉输入的模型")
	}
	payload := map[string]any{"model": model.Model, "messages": request.Messages, "stream": true}
	if len(request.Tools) > 0 {
		payload["tools"] = request.Tools
		payload["tool_choice"] = "auto"
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(model.BaseURL, "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+model.APIKey)
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return nil, apperror.Wrap("AI_NETWORK_ERROR", "AI 服务连接失败", "ai.provider", true, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return nil, apperror.New("AI_PROVIDER_ERROR", "AI 服务返回错误", "ai.provider", response.StatusCode >= 500).
			WithSuggestion(strings.TrimSpace(string(detail)))
	}
	return response, nil
}

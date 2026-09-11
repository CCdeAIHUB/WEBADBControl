package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type AIModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	APIKey  string `json:"apiKey,omitempty"`
	Vision  bool   `json:"vision"`
}

type Settings struct {
	Theme          string    `json:"theme"`
	Language       string    `json:"language"`
	RefreshSeconds int       `json:"refreshSeconds"`
	ScreenFPS      int       `json:"screenFps"`
	AIModels       []AIModel `json:"aiModels"`
	DefaultModelID string    `json:"defaultModelId,omitempty"`
	RemoteEnabled  bool      `json:"remoteEnabled"`
	RemoteAddress  string    `json:"remoteAddress"`
	RemotePort     int       `json:"remotePort"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	data Settings
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, data: Settings{Theme: "system", Language: "zh-CN", RefreshSeconds: 5, ScreenFPS: 2, AIModels: []AIModel{}, RemoteAddress: "0.0.0.0", RemotePort: 45921}}
	encoded, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(encoded) > 0 {
		if err := json.Unmarshal(encoded, &store.data); err != nil {
			return nil, err
		}
	}
	if store.data.AIModels == nil {
		store.data.AIModels = []AIModel{}
	}
	return store, nil
}

func (s *Store) Get(includeSecrets bool) Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.data
	data.AIModels = append([]AIModel{}, s.data.AIModels...)
	if !includeSecrets {
		for index := range data.AIModels {
			if data.AIModels[index].APIKey != "" {
				data.AIModels[index].APIKey = "••••••••"
			}
		}
	}
	return data
}

func (s *Store) Save(data Settings) error {
	if data.RefreshSeconds < 1 {
		data.RefreshSeconds = 5
	}
	if data.ScreenFPS < 1 || data.ScreenFPS > 10 {
		data.ScreenFPS = 2
	}
	if data.RemoteAddress == "" {
		data.RemoteAddress = "0.0.0.0"
	}
	if data.RemotePort < 1024 || data.RemotePort > 65535 {
		return errors.New("remote port must be between 1024 and 65535")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A masked key means the UI did not intend to replace the stored credential.
	for index := range data.AIModels {
		if data.AIModels[index].APIKey != "••••••••" {
			continue
		}
		for _, existing := range s.data.AIModels {
			if existing.ID == data.AIModels[index].ID {
				data.AIModels[index].APIKey = existing.APIKey
			}
		}
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return err
	}
	s.data = data
	return nil
}

func (s *Store) Model(id string) (AIModel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id == "" {
		id = s.data.DefaultModelID
	}
	for _, model := range s.data.AIModels {
		if model.ID == id {
			return model, true
		}
	}
	return AIModel{}, false
}

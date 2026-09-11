package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenNormalizesMissingAIModelsToEmptyArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"system","remotePort":45921}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if models := store.Get(false).AIModels; models == nil || len(models) != 0 {
		t.Fatalf("AI models must be a non-nil empty array, got %#v", models)
	}
}

package device

import "testing"

func TestCompanionInvocationResultSupportsDirectAndNestedCoreResponses(t *testing.T) {
	direct := companionInvocationResult(map[string]any{"result": map[string]any{"pngBase64": "direct"}})
	if direct["pngBase64"] != "direct" {
		t.Fatalf("direct result was not decoded: %#v", direct)
	}
	nested := companionInvocationResult(map[string]any{
		"envelope": map[string]any{
			"payload": map[string]any{
				"result": map[string]any{"pngBase64": "nested"},
			},
		},
	})
	if nested["pngBase64"] != "nested" {
		t.Fatalf("nested result was not decoded: %#v", nested)
	}
}

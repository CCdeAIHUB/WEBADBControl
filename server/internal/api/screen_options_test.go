package api

import "testing"

func TestScreenFrameRateUsesBoundedRequestOverride(t *testing.T) {
	// 场景：实时画面可独立选择帧率，但不得突破截图通道的安全上限。
	for _, test := range []struct {
		query      string
		configured int
		want       int
	}{
		{"", 2, 2},
		{"5", 2, 5},
		{"0", 2, 2},
		{"30", 2, 10},
		{"invalid", 2, 2},
	} {
		if got := screenFrameRate(test.query, test.configured); got != test.want {
			t.Fatalf("screenFrameRate(%q, %d) = %d, want %d", test.query, test.configured, got, test.want)
		}
	}
}

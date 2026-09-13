package api

import "testing"

func TestScreenFrameRateUsesBoundedRequestOverride(t *testing.T) {
	// 场景：实时视频可独立选择帧率，但不得突破 scrcpy 的安全上限。
	for _, test := range []struct {
		query      string
		configured int
		want       int
	}{
		{"", 30, 30},
		{"15", 30, 15},
		{"0", 30, 30},
		{"120", 30, 60},
		{"invalid", 30, 30},
	} {
		if got := screenFrameRate(test.query, test.configured); got != test.want {
			t.Fatalf("screenFrameRate(%q, %d) = %d, want %d", test.query, test.configured, got, test.want)
		}
	}
}

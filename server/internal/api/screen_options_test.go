package api

import (
	"encoding/binary"
	"testing"
)

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

func TestScreenPacketV2CarriesScrcpyPresentationTimestamp(t *testing.T) {
	// 场景：新页面协商 v2 后收到 kind + PTS + H.264；未协商的旧页面仍保持 kind + H.264。
	payload := []byte{0x00, 0x00, 0x01, 0x65}
	v2 := encodeScreenPacket(2, 2, 9_876_543, payload)
	if len(v2) != 9+len(payload) || v2[0] != 2 || binary.BigEndian.Uint64(v2[1:9]) != 9_876_543 {
		t.Fatalf("bad v2 packet: %x", v2)
	}
	v1 := encodeScreenPacket(1, 2, 9_876_543, payload)
	if len(v1) != 1+len(payload) || v1[0] != 2 || v1[1] != payload[0] {
		t.Fatalf("v1 compatibility changed: %x", v1)
	}
}

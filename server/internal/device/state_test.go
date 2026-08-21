package device

import "testing"

func TestParseLockStateRecognizesOEMFormats(t *testing.T) {
	// 场景：AOSP 与常见中国厂商 ROM 的锁屏字段都必须被识别，未知输出不得猜测。
	cases := []struct {
		name   string
		output string
		want   LockState
	}{
		{"aosp locked", "mShowingLockscreen=true\nmScreenOnFully=true", LockState{Locked: true, Awake: true, Known: true}},
		{"miui unlocked", "deviceLocked=0\ninteractiveState=AWAKE", LockState{Locked: false, Awake: true, Known: true}},
		{"coloros locked", "showing=true\nisInteractive=false", LockState{Locked: true, Awake: false, Known: true}},
		{"unknown", "window policy output without state", LockState{Known: false}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseLockState(test.output); got != test.want {
				t.Fatalf("got %#v want %#v", got, test.want)
			}
		})
	}
}

func TestParseHardwareSnapshotKeepsCoreMetrics(t *testing.T) {
	// 场景：硬件采集必须保留 CPU、内存、存储和温度，缺失字段不伪造数值。
	raw := "CPU=1800000,1200000\nMEM=MemTotal: 8000000 kB|MemAvailable: 3000000 kB\nDISK=/dev 100000 40000 60000 40% /data\nTEMP=cpu-0:42000,battery:315\nUPTIME=12345.67 100.00"
	snapshot := ParseHardwareSnapshot(raw)
	if len(snapshot.CPUFrequenciesKHz) != 2 || snapshot.MemoryTotalKB != 8_000_000 || snapshot.MemoryAvailableKB != 3_000_000 {
		t.Fatalf("unexpected core metrics: %#v", snapshot)
	}
	if snapshot.TemperaturesC["cpu-0"] != 42 || snapshot.TemperaturesC["battery"] != 31.5 {
		t.Fatalf("unexpected temperatures: %#v", snapshot.TemperaturesC)
	}
	if snapshot.UptimeSeconds != 12345.67 {
		t.Fatalf("unexpected uptime %f", snapshot.UptimeSeconds)
	}
}

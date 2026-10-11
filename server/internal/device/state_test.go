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
		{"coloros locked", "KeyguardServiceDelegate showing=true\nisInteractive=false", LockState{Locked: true, Awake: false, Known: true}},
		{"samsung external display unlocked", "Display 2 showing=true\nWindowState showing=true\nisKeyguardLocked=false\nisInteractive=true", LockState{Locked: false, Awake: true, Known: true}},
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

func TestParseHardwareSnapshotKeepsDesktopAlignedMetrics(t *testing.T) {
	raw := "brand=Samsung\nmodel=SM-F926N\ndevice=q2q\nandroid=14\nsdk=34\nabi=arm64-v8a\n" +
		"cpu_model=Snapdragon 888\ncpu_cores=8\ncpu_freqs=cpu0:1800000,cpu1:2200000,\ncpu_max_freqs=cpu0:2840000,cpu1:2840000,\n" +
		"battery_level=76\nbattery_status=2\nbattery_temp=315\nswap_total_kb=4194304\nswap_free_kb=1048576\nzram_disk_bytes=4294967296\n" +
		"load=1.20 0.80 0.40\nthermal_service=CPU:42.5,GPU:39000,\ngpu_access=available\ngpu_usage=37.5\ngpu_cur_freq=500000\ngpu_max_freq=840000000\ngpu_memory_bytes=268435456\nrefresh_rate=120"
	snapshot := ParseHardwareSnapshot(raw)
	if snapshot.Brand != "Samsung" || snapshot.Model != "SM-F926N" || snapshot.CPUCores != 8 {
		t.Fatalf("unexpected identity metrics: %#v", snapshot)
	}
	if len(snapshot.CPUMaxFrequenciesKHz) != 2 || snapshot.BatteryTemperatureC != 31.5 || snapshot.SwapTotalKB != 4_194_304 {
		t.Fatalf("unexpected resource metrics: %#v", snapshot)
	}
	if snapshot.GPUCurrentFrequencyHz != 500_000_000 || snapshot.GPUMaxFrequencyHz != 840_000_000 || snapshot.RefreshRateHz != 120 {
		t.Fatalf("unexpected gpu/display metrics: %#v", snapshot)
	}
	if snapshot.TemperaturesC["CPU"] != 42.5 || snapshot.TemperaturesC["GPU"] != 39 {
		t.Fatalf("unexpected extended temperatures: %#v", snapshot.TemperaturesC)
	}
}

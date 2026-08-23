package device

import "testing"

func TestParseFileListingReturnsStructuredEntries(t *testing.T) {
	// 场景：文件管理 UI 需要结构化目录项，而不是直接展示 ls 原始文本。
	output := `
total 12
drwxrwx--x 2 root sdcard_rw 4096 2026-08-23 15:00 Download
-rw-rw---- 1 root sdcard_rw 128 2026-08-23 15:01 report.txt
lrwxrwxrwx 1 root root 11 2026-08-23 15:02 link -> /sdcard/DCIM
`
	entries := ParseFileListing("/sdcard", output)
	if len(entries) != 3 {
		t.Fatalf("entry count = %d", len(entries))
	}
	if entries[0].Type != "directory" || entries[0].Path != "/sdcard/Download" {
		t.Fatalf("bad directory entry: %#v", entries[0])
	}
	if entries[1].Type != "file" || entries[1].Size != 128 {
		t.Fatalf("bad file entry: %#v", entries[1])
	}
	if entries[2].Type != "link" || entries[2].Target != "/sdcard/DCIM" {
		t.Fatalf("bad link entry: %#v", entries[2])
	}
}

func TestParseFileListingAcceptsAndroidShortDateVariant(t *testing.T) {
	// 场景：部分 Android ls 只返回日期不返回时间，仍然要提取正确文件名。
	entries := ParseFileListing("/sdcard", "-rw-rw---- 1 root sdcard_rw 128 2026-08-23 带 空格.txt\n")
	if len(entries) != 1 {
		t.Fatalf("entry count = %d", len(entries))
	}
	if entries[0].Name != "带 空格.txt" || entries[0].Modified != "2026-08-23" {
		t.Fatalf("bad short-date entry: %#v", entries[0])
	}
}

func TestValidateMutableRemotePathLimitsDangerousLocations(t *testing.T) {
	// 场景：删除/新建目录只允许共享存储下的具体路径，不能误删设备系统目录或存储根。
	if err := ValidateMutableRemotePath("/sdcard/Documents"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUploadDirectory("/sdcard"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/sdcard", "/system/bin/app_process"} {
		if err := ValidateMutableRemotePath(path); err == nil {
			t.Fatalf("expected %s to be rejected", path)
		}
	}
}

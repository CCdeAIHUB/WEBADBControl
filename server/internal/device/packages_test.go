package device

import "testing"

func TestParsePackageListReturnsDisplayMetadata(t *testing.T) {
	// 场景：应用管理页需要名称、APK 路径和版本信息，不能只显示裸包名。
	output := `package:/data/app/~~abc/com.example.cool_app/base.apk=com.example.cool_app versionCode:42
package:com.simple.reader
`
	apps := ParsePackageList(output)
	if len(apps) != 2 {
		t.Fatalf("apps = %d", len(apps))
	}
	if apps[0].Package != "com.example.cool_app" || apps[0].DisplayName != "Cool App" || apps[0].VersionCode != 42 || apps[0].APKPath == "" {
		t.Fatalf("bad first app: %#v", apps[0])
	}
	if apps[1].DisplayName != "Reader" || apps[1].IconText == "" || apps[1].IconColor == "" {
		t.Fatalf("bad fallback app: %#v", apps[1])
	}
}

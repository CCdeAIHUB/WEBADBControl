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

func TestParsePackageListKeepsSystemAndUserPackages(t *testing.T) {
	// 场景：应用管理必须显示完整应用目录，不能只显示第三方应用。
	output := `package:/system/priv-app/Settings/Settings.apk=com.android.settings versionCode:36
package:/data/app/~~abc/com.example.user/base.apk=com.example.user versionCode:7
`
	apps := ParsePackageList(output)
	if len(apps) != 2 {
		t.Fatalf("apps = %d, want 2", len(apps))
	}
	if !apps[0].System || apps[1].System {
		t.Fatalf("system flags = %#v %#v", apps[0], apps[1])
	}
}

func TestParsePackageLabelsAndApplyResolvedNames(t *testing.T) {
	// 场景：设备提供 application-label 时，Web 版应优先显示本机应用名称。
	apps := []PackageInfo{{Package: "com.android.settings", DisplayName: "Settings", IconText: "SE"}}
	labels := ParsePackageLabels("Package [com.android.settings]\n  application-label-zh-CN:'设置'\n")
	ApplyPackageLabels(apps, labels)
	if apps[0].DisplayName != "设置" || !apps[0].HasResolvedDisplayName || apps[0].IconText != "设置" {
		t.Fatalf("bad resolved app: %#v", apps[0])
	}
}

func TestApplyCompanionPackageMetadataAddsRealLabelAndIcon(t *testing.T) {
	// 场景：伴侣 App 返回本机 PackageManager 元数据时，应用管理必须显示真实名称和 PNG 图标。
	payload := `{"ok":true,"result":{"apps":[{"packageName":"com.example.user","label":"示例应用","enabled":true,"system":false,"sourceDir":"/data/app/app.apk","iconPngBase64":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"}]}}`
	metadata, err := ParseCompanionPackageMetadata(payload)
	if err != nil {
		t.Fatal(err)
	}
	apps := []PackageInfo{{Package: "com.example.user", DisplayName: "User", IconText: "US"}}
	ApplyCompanionPackageMetadata(apps, metadata)
	if apps[0].DisplayName != "示例应用" || !apps[0].HasResolvedDisplayName || apps[0].IconPngBase64 == "" || apps[0].System {
		t.Fatalf("bad companion metadata: %#v", apps[0])
	}
	if apps[0].Enabled == nil || !*apps[0].Enabled {
		t.Fatalf("enabled flag not applied: %#v", apps[0])
	}
}

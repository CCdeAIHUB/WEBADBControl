package device

import (
	"context"
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type PackageInfo struct {
	Package                string `json:"package"`
	DisplayName            string `json:"displayName"`
	HasResolvedDisplayName bool   `json:"hasResolvedDisplayName"`
	APKPath                string `json:"apkPath,omitempty"`
	VersionCode            int64  `json:"versionCode,omitempty"`
	System                 bool   `json:"system"`
	Enabled                *bool  `json:"enabled,omitempty"`
	IconPngBase64          string `json:"iconPngBase64,omitempty"`
	MetadataSource         string `json:"metadataSource"`
	IconText               string `json:"iconText"`
	IconColor              string `json:"iconColor"`
}

var (
	packageNamePattern        = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+$`)
	packageLabelHeaderPattern = regexp.MustCompile(`^Package \[([^\]]+)\]`)
)

func (s *Service) Packages(ctx context.Context, deviceID string) ([]PackageInfo, error) {
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "list", "packages", "-f", "--show-versioncode"))
	if err != nil {
		return nil, err
	}
	packages := ParsePackageList(output.Stdout)
	dump, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "dumpsys", "package"))
	if err == nil {
		ApplyPackageLabels(packages, ParsePackageLabels(dump.Stdout))
	}
	_ = s.applyCompanionPackageMetadata(ctx, deviceID, packages)
	return packages, nil
}

func (s *Service) applyCompanionPackageMetadata(ctx context.Context, deviceID string, packages []PackageInfo) error {
	for offset := 0; offset < len(packages); offset += 64 {
		end := offset + 64
		if end > len(packages) {
			end = len(packages)
		}
		names := make([]string, 0, end-offset)
		for _, app := range packages[offset:end] {
			names = append(names, app.Package)
		}
		payload, err := s.ExecuteCompanionCommand(ctx, deviceID, "android.app.list", "app.list", map[string]any{
			"includeSystem": true,
			"includeIcons":  true,
			"iconSizePx":    48,
			"offset":        0,
			"limit":         len(names),
			"packageNames":  names,
		})
		if err != nil {
			return err
		}
		page, err := ParseCompanionPackageMetadata(payload)
		if err != nil {
			return err
		}
		ApplyCompanionPackageMetadata(packages, page)
	}
	return nil
}

func ParsePackageList(output string) []PackageInfo {
	apps := make([]PackageInfo, 0)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		info, ok := parsePackageLine(line)
		if ok {
			apps = append(apps, info)
		}
	}
	return apps
}

func parsePackageLine(line string) (PackageInfo, bool) {
	line = strings.TrimPrefix(line, "package:")
	versionCode := int64(0)
	versionPattern := regexp.MustCompile(`\s+versionCode:(\d+)`)
	if matches := versionPattern.FindStringSubmatch(line); len(matches) == 2 {
		versionCode, _ = strconv.ParseInt(matches[1], 10, 64)
		line = versionPattern.ReplaceAllString(line, "")
	}
	apkPath := ""
	packageName := line
	if left, right, ok := strings.Cut(line, "="); ok {
		apkPath, packageName = left, right
	}
	packageName = strings.TrimSpace(packageName)
	if fields := strings.Fields(packageName); len(fields) > 0 {
		packageName = fields[0]
	}
	if !packageNamePattern.MatchString(packageName) {
		return PackageInfo{}, false
	}
	return PackageInfo{
		Package:        packageName,
		DisplayName:    displayNameFromPackage(packageName),
		APKPath:        strings.TrimSpace(apkPath),
		VersionCode:    versionCode,
		System:         isSystemAPK(apkPath),
		MetadataSource: "package-name",
		IconText:       iconText(packageName),
		IconColor:      iconColor(packageName),
	}, true
}

func isSystemAPK(apkPath string) bool {
	apkPath = strings.TrimSpace(apkPath)
	return strings.HasPrefix(apkPath, "/system/") ||
		strings.HasPrefix(apkPath, "/product/") ||
		strings.HasPrefix(apkPath, "/vendor/") ||
		strings.HasPrefix(apkPath, "/apex/") ||
		strings.HasPrefix(apkPath, "/system_ext/")
}

func ApplyPackageLabels(packages []PackageInfo, labels map[string]string) {
	for index := range packages {
		label := strings.TrimSpace(labels[packages[index].Package])
		if label == "" {
			continue
		}
		packages[index].DisplayName = label
		packages[index].HasResolvedDisplayName = true
		packages[index].MetadataSource = "adb-label"
		packages[index].IconText = iconTextFromDisplay(label)
	}
}

func ParsePackageLabels(output string) map[string]string {
	labels := map[string]string{}
	currentPackage := ""
	for _, rawLine := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if matches := packageLabelHeaderPattern.FindStringSubmatch(line); len(matches) == 2 {
			currentPackage = matches[1]
			continue
		}
		if currentPackage == "" || !strings.HasPrefix(strings.ToLower(line), "application-label") {
			continue
		}
		_, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		label := strings.TrimSpace(value)
		if index := strings.LastIndex(label, "="); index >= 0 {
			label = label[index+1:]
		}
		label = strings.Trim(strings.TrimSpace(label), `"'`)
		if label != "" {
			labels[currentPackage] = label
		}
	}
	return labels
}

type CompanionPackageMetadata struct {
	PackageName   string `json:"packageName"`
	Label         string `json:"label"`
	Enabled       *bool  `json:"enabled"`
	System        *bool  `json:"system"`
	SourceDir     string `json:"sourceDir"`
	IconPngBase64 string `json:"iconPngBase64"`
}

type companionPackagePage struct {
	OK     *bool `json:"ok"`
	Result struct {
		Apps []CompanionPackageMetadata `json:"apps"`
	} `json:"result"`
	Apps []CompanionPackageMetadata `json:"apps"`
}

func ParseCompanionPackageMetadata(payload string) ([]CompanionPackageMetadata, error) {
	var page companionPackagePage
	if err := json.Unmarshal([]byte(payload), &page); err != nil {
		return nil, err
	}
	if page.OK != nil && !*page.OK {
		return []CompanionPackageMetadata{}, nil
	}
	if len(page.Result.Apps) > 0 {
		return page.Result.Apps, nil
	}
	return page.Apps, nil
}

func ApplyCompanionPackageMetadata(packages []PackageInfo, metadata []CompanionPackageMetadata) {
	indexByPackage := make(map[string]int, len(packages))
	for index, app := range packages {
		indexByPackage[strings.ToLower(app.Package)] = index
	}
	for _, item := range metadata {
		index, exists := indexByPackage[strings.ToLower(item.PackageName)]
		if !exists {
			continue
		}
		if strings.TrimSpace(item.Label) != "" {
			packages[index].DisplayName = item.Label
			packages[index].HasResolvedDisplayName = true
			packages[index].MetadataSource = "companion"
			packages[index].IconText = iconTextFromDisplay(item.Label)
		}
		if item.SourceDir != "" {
			packages[index].APKPath = item.SourceDir
		}
		if item.System != nil {
			packages[index].System = *item.System
		}
		if item.Enabled != nil {
			packages[index].Enabled = item.Enabled
		}
		if validPngBase64(item.IconPngBase64) {
			packages[index].IconPngBase64 = item.IconPngBase64
			packages[index].MetadataSource = "companion"
		}
	}
}

func validPngBase64(encoded string) bool {
	if encoded == "" || len(encoded) > 350_000 {
		return false
	}
	return strings.HasPrefix(encoded, "iVBORw0KGgo")
}

func displayNameFromPackage(packageName string) string {
	parts := strings.Split(packageName, ".")
	name := parts[len(parts)-1]
	if name == "" && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	name = strings.NewReplacer("_", " ", "-", " ").Replace(name)
	words := strings.Fields(name)
	for index, word := range words {
		if len(word) > 0 {
			words[index] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	if len(words) == 0 {
		return path.Base(packageName)
	}
	return strings.Join(words, " ")
}

func iconText(packageName string) string {
	return iconTextFromDisplay(displayNameFromPackage(packageName))
}

func iconTextFromDisplay(display string) string {
	if display == "" {
		return "APP"
	}
	runes := []rune(display)
	if len(runes) == 1 {
		return strings.ToUpper(string(runes[0]))
	}
	return strings.ToUpper(string(runes[:min(2, len(runes))]))
}

func iconColor(packageName string) string {
	palette := []string{"emerald", "sky", "violet", "amber", "rose", "cyan", "lime", "indigo"}
	sum := 0
	for _, value := range packageName {
		sum += int(value)
	}
	return palette[sum%len(palette)]
}

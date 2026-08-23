package device

import (
	"context"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type PackageInfo struct {
	Package     string `json:"package"`
	DisplayName string `json:"displayName"`
	APKPath     string `json:"apkPath,omitempty"`
	VersionCode int64  `json:"versionCode,omitempty"`
	IconText    string `json:"iconText"`
	IconColor   string `json:"iconColor"`
}

var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+$`)

func (s *Service) Packages(ctx context.Context, deviceID string) ([]PackageInfo, error) {
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "list", "packages", "-3", "-f", "--show-versioncode"))
	if err != nil {
		return nil, err
	}
	return ParsePackageList(output.Stdout), nil
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
		Package:     packageName,
		DisplayName: displayNameFromPackage(packageName),
		APKPath:     strings.TrimSpace(apkPath),
		VersionCode: versionCode,
		IconText:    iconText(packageName),
		IconColor:   iconColor(packageName),
	}, true
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
	display := displayNameFromPackage(packageName)
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

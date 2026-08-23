package device

import (
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

func adbCommandFailure(args []string, output CommandOutput) error {
	detail := strings.TrimSpace(strings.Join([]string{output.Stderr, output.Stdout}, "\n"))
	command := ""
	if len(args) > 0 {
		command = args[0]
	}

	// Platform-Tools before Android 11 may not implement pair/mdns. Preserve a
	// dedicated compatibility error so deployment problems are never reported as bad user input.
	if strings.Contains(strings.ToLower(detail), "unknown command") {
		switch command {
		case "pair":
			return apperror.New("ADB_PAIR_UNSUPPORTED", "当前 ADB 版本不支持六位码配对", "device.connection", false).
				WithSuggestion("请升级服务端 Android Platform-Tools 后重试")
		case "mdns":
			return apperror.New("ADB_MDNS_UNSUPPORTED", "当前 ADB 版本不支持无线设备发现", "device.connection", false).
				WithSuggestion("请升级服务端 Android Platform-Tools；升级前仍可手工输入设备地址")
		}
	}

	return apperror.New("ADB_COMMAND_FAILED", "设备命令执行失败", "device.service", true).
		WithSuggestion(detail)
}

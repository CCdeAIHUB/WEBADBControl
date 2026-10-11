package device

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type LockState struct {
	Locked bool `json:"locked"`
	Awake  bool `json:"awake"`
	Known  bool `json:"known"`
}

func ParseLockState(output string) LockState {
	normalized := strings.ToLower(strings.ReplaceAll(output, " ", ""))
	state := LockState{}
	for _, marker := range []string{"mshowinglockscreen=true", "devicelocked=1", "devicelocked=true", "isstatusbarkeyguard=true", "keyguardshowing=true", "iskeyguardlocked=true"} {
		if strings.Contains(normalized, marker) {
			state.Locked, state.Known = true, true
			break
		}
	}
	if !state.Known {
		for _, marker := range []string{"mshowinglockscreen=false", "devicelocked=0", "devicelocked=false", "isstatusbarkeyguard=false", "keyguardshowing=false", "iskeyguardlocked=false"} {
			if strings.Contains(normalized, marker) {
				state.Known = true
				break
			}
		}
	}
	if !state.Known {
		state = parseContextualKeyguardShowing(output, state)
	}
	for _, marker := range []string{"mscreenonfully=true", "interactivestate=awake", "mwakefulness=awake", "isinteractive=true", "displaypowerstate=on"} {
		if strings.Contains(normalized, marker) {
			state.Awake = true
			break
		}
	}
	return state
}

func parseContextualKeyguardShowing(output string, state LockState) LockState {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		normalizedLine := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(line), " ", ""))
		if !strings.Contains(normalizedLine, "showing=") {
			continue
		}
		// Samsung foldables with external displays can include unrelated Window/Display
		// "showing=true" fields while the keyguard is not active. Treat generic
		// showing as lock state only when the same line explicitly belongs to keyguard
		// or lockscreen policy output.
		if !strings.Contains(normalizedLine, "keyguard") && !strings.Contains(normalizedLine, "lockscreen") {
			continue
		}
		if strings.Contains(normalizedLine, "showing=true") {
			state.Locked, state.Known = true, true
			return state
		}
		if strings.Contains(normalizedLine, "showing=false") {
			state.Known = true
			return state
		}
	}
	return state
}

type HardwareSnapshot struct {
	CapturedAt            time.Time          `json:"capturedAt"`
	CPUFrequenciesKHz     []int64            `json:"cpuFrequenciesKHz"`
	MemoryTotalKB         int64              `json:"memoryTotalKb"`
	MemoryAvailableKB     int64              `json:"memoryAvailableKb"`
	StorageTotalKB        int64              `json:"storageTotalKb"`
	StorageUsedKB         int64              `json:"storageUsedKb"`
	TemperaturesC         map[string]float64 `json:"temperaturesC"`
	UptimeSeconds         float64            `json:"uptimeSeconds"`
	Brand                 string             `json:"brand"`
	Model                 string             `json:"model"`
	Device                string             `json:"device"`
	AndroidVersion        string             `json:"androidVersion"`
	SDK                   string             `json:"sdk"`
	ABI                   string             `json:"abi"`
	CPUModel              string             `json:"cpuModel"`
	CPUCores              int                `json:"cpuCores"`
	CPUMaxFrequenciesKHz  []int64            `json:"cpuMaxFrequenciesKHz"`
	BatteryLevel          int                `json:"batteryLevel"`
	BatteryStatus         int                `json:"batteryStatus"`
	BatteryTemperatureC   float64            `json:"batteryTemperatureC"`
	SwapTotalKB           int64              `json:"swapTotalKb"`
	SwapFreeKB            int64              `json:"swapFreeKb"`
	ZramDiskBytes         int64              `json:"zramDiskBytes"`
	LoadAverage           string             `json:"loadAverage"`
	GPUAccess             string             `json:"gpuAccess"`
	GPUUsagePercent       float64            `json:"gpuUsagePercent"`
	GPUCurrentFrequencyHz int64              `json:"gpuCurrentFrequencyHz"`
	GPUMaxFrequencyHz     int64              `json:"gpuMaxFrequencyHz"`
	GPUMemoryBytes        int64              `json:"gpuMemoryBytes"`
	RefreshRateHz         float64            `json:"refreshRateHz"`
}

func ParseHardwareSnapshot(output string) HardwareSnapshot {
	snapshot := HardwareSnapshot{CapturedAt: time.Now().UTC(), TemperaturesC: map[string]float64{}}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "brand":
			snapshot.Brand = value
		case "model":
			snapshot.Model = value
		case "device":
			snapshot.Device = value
		case "android":
			snapshot.AndroidVersion = value
		case "sdk":
			snapshot.SDK = value
		case "abi":
			snapshot.ABI = value
		case "cpu_model":
			snapshot.CPUModel = value
		case "cpu_cores":
			snapshot.CPUCores, _ = strconv.Atoi(strings.TrimSpace(value))
		case "cpu_max_freqs":
			snapshot.CPUMaxFrequenciesKHz = parseNamedInt64s(value)
		case "battery_level":
			snapshot.BatteryLevel, _ = strconv.Atoi(strings.TrimSpace(value))
		case "battery_status":
			snapshot.BatteryStatus, _ = strconv.Atoi(strings.TrimSpace(value))
		case "battery_temp":
			raw, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
			snapshot.BatteryTemperatureC = raw / 10
		case "swap_total_kb":
			snapshot.SwapTotalKB, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "swap_free_kb":
			snapshot.SwapFreeKB, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "zram_disk_bytes":
			snapshot.ZramDiskBytes, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "load":
			snapshot.LoadAverage = value
		case "gpu_access":
			snapshot.GPUAccess = value
		case "gpu_usage":
			snapshot.GPUUsagePercent, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
		case "gpu_cur_freq":
			snapshot.GPUCurrentFrequencyHz = normalizeHardwareFrequency(value)
		case "gpu_max_freq":
			snapshot.GPUMaxFrequencyHz = normalizeHardwareFrequency(value)
		case "gpu_memory_bytes":
			snapshot.GPUMemoryBytes, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "refresh_rate":
			snapshot.RefreshRateHz, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
		case "cpu_freqs":
			snapshot.CPUFrequenciesKHz = parseNamedInt64s(value)
		case "mem_total_kb":
			snapshot.MemoryTotalKB, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "mem_available_kb":
			snapshot.MemoryAvailableKB, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "thermal_service", "temperatures":
			if value != "" {
				parseTemperaturesInto(snapshot.TemperaturesC, value)
			}
		case "CPU":
			for _, raw := range strings.Split(value, ",") {
				if frequency, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && frequency > 0 {
					snapshot.CPUFrequenciesKHz = append(snapshot.CPUFrequenciesKHz, frequency)
				}
			}
		case "MEM":
			for _, item := range strings.Split(value, "|") {
				fields := strings.Fields(item)
				if len(fields) < 2 {
					continue
				}
				number, _ := strconv.ParseInt(fields[1], 10, 64)
				if strings.HasPrefix(fields[0], "MemTotal") {
					snapshot.MemoryTotalKB = number
				} else if strings.HasPrefix(fields[0], "MemAvailable") {
					snapshot.MemoryAvailableKB = number
				}
			}
		case "DISK":
			fields := strings.Fields(value)
			if len(fields) >= 4 {
				snapshot.StorageTotalKB, _ = strconv.ParseInt(fields[1], 10, 64)
				snapshot.StorageUsedKB, _ = strconv.ParseInt(fields[2], 10, 64)
			}
		case "TEMP":
			for _, item := range strings.Split(value, ",") {
				name, raw, ok := strings.Cut(strings.TrimSpace(item), ":")
				if !ok || name == "" {
					continue
				}
				temperature, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
				if err != nil {
					continue
				}
				if temperature > 1000 {
					temperature /= 1000
				} else if temperature > 100 {
					temperature /= 10
				}
				snapshot.TemperaturesC[name] = temperature
			}
		case "UPTIME":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				snapshot.UptimeSeconds, _ = strconv.ParseFloat(fields[0], 64)
			}
		}
	}
	return snapshot
}

func parseNamedInt64s(value string) []int64 {
	result := []int64{}
	for _, item := range strings.Split(value, ",") {
		_, raw, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		if number, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && number > 0 {
			result = append(result, number)
		}
	}
	return result
}

func parseTemperaturesInto(target map[string]float64, value string) {
	for _, item := range strings.Split(value, ",") {
		name, raw, ok := strings.Cut(item, ":")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		temperature, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			continue
		}
		if temperature > 200 {
			temperature /= 1000
		}
		target[strings.TrimSpace(name)] = temperature
	}
}

func normalizeHardwareFrequency(value string) int64 {
	number, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if number > 0 && number < 10_000_000 {
		number *= 1000
	}
	return number
}

type DiscoveredService struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Endpoint string `json:"endpoint"`
}

func ParseMDNS(output string) []DiscoveredService {
	services := make([]DiscoveredService, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		fields := strings.Fields(line)
		for _, field := range fields {
			candidate := strings.Trim(field, ",;")
			if _, _, err := net.SplitHostPort(candidate); err != nil {
				continue
			}
			normalized, err := normalizeWirelessEndpoint(candidate)
			if err != nil {
				continue
			}
			serviceType := "connect"
			if strings.Contains(line, "_adb-tls-pairing") {
				serviceType = "pairing"
			}
			key := serviceType + "|" + normalized
			if _, exists := seen[key]; exists {
				break
			}
			seen[key] = struct{}{}
			name := normalized
			if len(fields) > 0 {
				name = fields[0]
			}
			services = append(services, DiscoveredService{Name: name, Type: serviceType, Endpoint: normalized})
			break
		}
	}
	return services
}

func parseScreenSize(output string) (int, int, error) {
	pattern := regexp.MustCompile(`(?m)(?:Physical|Override) size:\s*(\d+)x(\d+)`)
	matches := pattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, 0, fmt.Errorf("screen size was not reported")
	}
	match := matches[len(matches)-1]
	width, _ := strconv.Atoi(match[1])
	height, _ := strconv.Atoi(match[2])
	return width, height, nil
}

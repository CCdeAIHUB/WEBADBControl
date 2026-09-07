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
	CapturedAt        time.Time          `json:"capturedAt"`
	CPUFrequenciesKHz []int64            `json:"cpuFrequenciesKHz"`
	MemoryTotalKB     int64              `json:"memoryTotalKb"`
	MemoryAvailableKB int64              `json:"memoryAvailableKb"`
	StorageTotalKB    int64              `json:"storageTotalKb"`
	StorageUsedKB     int64              `json:"storageUsedKb"`
	TemperaturesC     map[string]float64 `json:"temperaturesC"`
	UptimeSeconds     float64            `json:"uptimeSeconds"`
}

func ParseHardwareSnapshot(output string) HardwareSnapshot {
	snapshot := HardwareSnapshot{CapturedAt: time.Now().UTC(), TemperaturesC: map[string]float64{}}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
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

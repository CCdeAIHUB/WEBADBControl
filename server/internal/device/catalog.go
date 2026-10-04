package device

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
)

// Catalog is the persistent device directory. ADB connectivity is transient;
// discovering a handset once must not make it disappear from management when
// wireless debugging temporarily disconnects.
type Catalog struct {
	mu      sync.Mutex
	path    string
	devices []Device
}

func newMemoryCatalog() *Catalog { return &Catalog{} }

func OpenCatalog(path string) (*Catalog, error) {
	catalog := &Catalog{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) != 0 {
		if err := json.Unmarshal(data, &catalog.devices); err != nil {
			return nil, err
		}
	}
	for index := range catalog.devices {
		catalog.devices[index].State = "offline"
	}
	return catalog, nil
}

func (c *Catalog) Merge(live []Device) ([]Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := cloneDevices(c.devices)
	matched := make([]bool, len(live))
	for rememberedIndex, remembered := range result {
		result[rememberedIndex].State = "offline"
		for liveIndex, current := range live {
			if matched[liveIndex] || !sameCatalogDevice(remembered, current) {
				continue
			}
			merged := current
			if merged.Name == "" || merged.Name == merged.ID {
				if remembered.Name != "" && remembered.Name != remembered.ID {
					merged.Name = remembered.Name
				}
			}
			if merged.Model == "" {
				merged.Model = remembered.Model
			}
			if merged.Product == "" {
				merged.Product = remembered.Product
			}
			if merged.HardwareID == "" {
				merged.HardwareID = remembered.HardwareID
			}
			merged.Aliases = appendUnique(merged.Aliases, remembered.ID)
			merged.Aliases = appendUnique(merged.Aliases, remembered.Aliases...)
			merged.Aliases = aliasesWithoutPrimary(merged.ID, merged.Aliases)
			result[rememberedIndex] = merged
			matched[liveIndex] = true
			break
		}
	}
	for index, current := range live {
		if !matched[index] {
			result = append(result, current)
		}
	}

	stored := cloneDevices(result)
	for index := range stored {
		stored[index].State = "offline"
	}
	if !reflect.DeepEqual(stored, c.devices) {
		if err := c.saveLocked(stored); err != nil {
			return nil, err
		}
		c.devices = stored
	}
	return cloneDevices(result), nil
}

func (c *Catalog) Remove(deviceID string) (Device, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, candidate := range c.devices {
		if candidate.ID != deviceID && !containsDeviceID(candidate.Aliases, deviceID) && !EquivalentDeviceID(candidate.ID, deviceID) {
			continue
		}
		remaining := append(cloneDevices(c.devices[:index]), cloneDevices(c.devices[index+1:])...)
		if err := c.saveLocked(remaining); err != nil {
			return Device{}, false, err
		}
		removed := candidate
		c.devices = remaining
		return removed, true, nil
	}
	return Device{}, false, nil
}

func (c *Catalog) saveLocked(devices []Device) error {
	if c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return err
	}
	temporary := c.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, c.path); err != nil {
		// Windows does not replace an existing destination with os.Rename.
		// Production Linux takes the atomic path above; this fallback keeps local
		// development and tests portable.
		if removeErr := os.Remove(c.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(temporary)
			return err
		}
		if retryErr := os.Rename(temporary, c.path); retryErr != nil {
			_ = os.Remove(temporary)
			return retryErr
		}
	}
	return nil
}

func sameCatalogDevice(left, right Device) bool {
	if left.HardwareID != "" && right.HardwareID != "" && left.HardwareID == right.HardwareID {
		return true
	}
	leftIDs := append([]string{left.ID}, left.Aliases...)
	rightIDs := append([]string{right.ID}, right.Aliases...)
	for _, leftID := range leftIDs {
		for _, rightID := range rightIDs {
			if EquivalentDeviceID(leftID, rightID) {
				return true
			}
		}
	}
	return false
}

func cloneDevices(devices []Device) []Device {
	result := make([]Device, len(devices))
	for index, candidate := range devices {
		result[index] = candidate
		result[index].Aliases = append([]string(nil), candidate.Aliases...)
	}
	return result
}

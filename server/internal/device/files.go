package device

import (
	"context"
	"fmt"
	pathpkg "path"
	"strconv"
	"strings"
)

type FileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner,omitempty"`
	Group       string `json:"group,omitempty"`
	Size        int64  `json:"size"`
	Modified    string `json:"modified,omitempty"`
	Target      string `json:"target,omitempty"`
}

func (s *Service) ListFiles(ctx context.Context, deviceID, remotePath string) ([]FileEntry, error) {
	if err := ValidateRemotePath(remotePath); err != nil {
		return nil, err
	}
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "ls", "-la", remotePath))
	if err != nil {
		return nil, err
	}
	return ParseFileListing(remotePath, output.Stdout), nil
}

func (s *Service) MakeDirectory(ctx context.Context, deviceID, remotePath string) error {
	if err := ValidateMutableRemotePath(remotePath); err != nil {
		return err
	}
	_, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "mkdir", "-p", remotePath))
	return err
}

func (s *Service) RemoveFile(ctx context.Context, deviceID, remotePath string) error {
	if err := ValidateMutableRemotePath(remotePath); err != nil {
		return err
	}
	_, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "rm", "-rf", remotePath))
	return err
}

func ParseFileListing(basePath, output string) []FileEntry {
	entries := make([]FileEntry, 0)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		entry, ok := parseFileEntry(basePath, line)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseFileEntry(basePath, line string) (FileEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 7 || fields[0] == "" {
		return FileEntry{}, false
	}
	ownerIndex := 1
	if _, err := strconv.Atoi(fields[1]); err == nil {
		ownerIndex = 2
	}
	sizeIndex := ownerIndex + 2
	modifiedStart := sizeIndex + 1
	modifiedEnd := modifiedStart + 1
	nameStart := modifiedEnd
	switch {
	case isISODate(fields[modifiedStart]) && len(fields) > modifiedStart+1 && looksLikeTimeOrYear(fields[modifiedStart+1]):
		// Android toybox commonly emits: perms links owner group size YYYY-MM-DD HH:MM name.
		// GNU coreutils commonly emits: perms links owner group size Mon DD HH:MM name.
		// Keep both forms explicit so names containing spaces remain intact.
		modifiedEnd = modifiedStart + 2
		nameStart = modifiedEnd
	case !isISODate(fields[modifiedStart]) && len(fields) > modifiedStart+3:
		modifiedEnd = modifiedStart + 3
		nameStart = modifiedEnd
	}
	if len(fields) <= nameStart {
		return FileEntry{}, false
	}
	name := strings.Join(fields[nameStart:], " ")
	if name == "." || name == ".." {
		return FileEntry{}, false
	}
	target := ""
	if linkName, linkTarget, ok := strings.Cut(name, " -> "); ok {
		name, target = linkName, linkTarget
	}
	entryType := "file"
	switch fields[0][0] {
	case 'd':
		entryType = "directory"
	case 'l':
		entryType = "link"
	}
	size, _ := strconv.ParseInt(fields[sizeIndex], 10, 64)
	return FileEntry{
		Name:        name,
		Path:        pathpkg.Join(basePath, name),
		Type:        entryType,
		Permissions: fields[0],
		Owner:       fields[ownerIndex],
		Group:       fields[ownerIndex+1],
		Size:        size,
		Modified:    strings.Join(fields[modifiedStart:modifiedEnd], " "),
		Target:      target,
	}, true
}

func isISODate(value string) bool {
	return len(value) == 10 && value[4] == '-' && value[7] == '-'
}

func looksLikeTimeOrYear(value string) bool {
	if len(value) == 4 {
		_, err := strconv.Atoi(value)
		return err == nil
	}
	return strings.Contains(value, ":")
}

func ValidateUploadDirectory(remotePath string) error {
	if err := ValidateRemotePath(remotePath); err != nil {
		return err
	}
	cleaned := pathpkg.Clean(remotePath)
	allowed := cleaned == "/sdcard" ||
		strings.HasPrefix(cleaned, "/sdcard/") ||
		cleaned == "/storage/emulated/0" ||
		strings.HasPrefix(cleaned, "/storage/emulated/0/")
	if !allowed {
		return fmt.Errorf("file upload is limited to shared storage")
	}
	return nil
}

func ValidateMutableRemotePath(remotePath string) error {
	if err := ValidateUploadDirectory(remotePath); err != nil {
		return err
	}
	cleaned := pathpkg.Clean(remotePath)
	if cleaned == "/sdcard" || cleaned == "/storage/emulated/0" {
		return fmt.Errorf("refusing to modify shared storage root")
	}
	return nil
}

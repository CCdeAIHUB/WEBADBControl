package device

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScrcpySession owns the local forwarded socket. Closing it always removes the
// ADB forward so an interrupted browser cannot leak a device-side session.
type ScrcpySession struct {
	service *Service
	deviceID string
	port     int
	video    net.Conn
	control  net.Conn
	controlMu sync.Mutex
	closeOnce sync.Once
	width    int
	height   int
}

func (s *Service) StartScrcpy(ctx context.Context, deviceID string) (*ScrcpySession, error) {
	pathOutput, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "path", companionPackageName))
	if err != nil { return nil, err }
	apkPath := ""
	for _, line := range strings.Split(pathOutput.Stdout, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok { apkPath = value; break }
	}
	if apkPath == "" { return nil, fmt.Errorf("ADBControl Companion is not installed") }
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil { return nil, err }
	scid := binary.BigEndian.Uint32(random[:]) | 0x10000000
	forward, err := s.Exec(ctx, DeviceArgs(deviceID, "forward", "tcp:0", fmt.Sprintf("localabstract:scrcpy_%08x", scid)))
	if err != nil { return nil, err }
	port, err := strconv.Atoi(strings.TrimSpace(forward.Stdout))
	if err != nil || port < 1 { return nil, fmt.Errorf("scrcpy forward did not return a port") }
	cleanup := func() { _, _ = s.Exec(context.Background(), DeviceArgs(deviceID, "forward", "--remove", fmt.Sprintf("tcp:%d", port))) }
	// The classpath originates from Android's package manager, never from the browser.
	command := fmt.Sprintf("CLASSPATH=%s setsid app_process / com.genymobile.scrcpy.Server 4.0 scid=%08x log_level=warn audio=false video=true control=true video_codec=h264 max_size=1280 video_bit_rate=4000000 max_fps=30 tunnel_forward=true send_device_meta=false send_dummy_byte=true send_stream_meta=true send_frame_meta=true </dev/null >/dev/null 2>&1 &", apkPath, scid)
	if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", command)); err != nil { cleanup(); return nil, err }
	deadline := time.Now().Add(8 * time.Second)
	var connection net.Conn
	for time.Now().Before(deadline) {
		connection, err = net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 600*time.Millisecond)
		if err == nil { break }
		time.Sleep(80 * time.Millisecond)
	}
	if err != nil { cleanup(); return nil, fmt.Errorf("scrcpy socket unavailable: %w", err) }
	dummy := make([]byte, 1)
	if err := readExactly(connection, dummy); err != nil { connection.Close(); cleanup(); return nil, fmt.Errorf("scrcpy handshake is invalid: %w", err) }
	control, controlErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if controlErr != nil { connection.Close(); cleanup(); return nil, fmt.Errorf("scrcpy control socket unavailable: %w", controlErr) }
	codec := make([]byte, 4)
	metadata := make([]byte, 12)
	if dummy[0] == 0 {
		if err := readExactly(connection, codec); err != nil { connection.Close(); control.Close(); cleanup(); return nil, err }
	} else {
		// Some companion builds ignore send_dummy_byte and start directly with
		// the codec id. Preserve that first byte instead of losing alignment.
		codec[0] = dummy[0]
		if err := readExactly(connection, codec[1:]); err != nil { connection.Close(); control.Close(); cleanup(); return nil, err }
	}
	if string(codec) != "h264" { connection.Close(); control.Close(); cleanup(); return nil, fmt.Errorf("scrcpy did not negotiate H.264 (codec=%q)", string(codec)) }
	if err := readExactly(connection, metadata); err != nil || metadata[0]&0x80 == 0 { connection.Close(); control.Close(); cleanup(); return nil, fmt.Errorf("scrcpy stream metadata is invalid") }
	width, height := int(binary.BigEndian.Uint32(metadata[4:8])), int(binary.BigEndian.Uint32(metadata[8:12]))
	if width < 1 || height < 1 { connection.Close(); cleanup(); return nil, fmt.Errorf("scrcpy frame size is invalid") }
	return &ScrcpySession{service:s, deviceID:deviceID, port:port, video:connection, control:control, width:width, height:height}, nil
}

func (s *ScrcpySession) Size() (int, int) { return s.width, s.height }
func (s *ScrcpySession) Close() { s.closeOnce.Do(func() { if s.video != nil { _ = s.video.Close() }; if s.control != nil { _ = s.control.Close() }; _, _ = s.service.Exec(context.Background(), DeviceArgs(s.deviceID, "forward", "--remove", fmt.Sprintf("tcp:%d", s.port))) }) }

func (s *ScrcpySession) SendTouch(action int, pointerID uint32, x, y, width, height int) error {
	message, err := EncodeScrcpyTouch(action, pointerID, x, y, width, height)
	if err != nil { return err }
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	_, err = s.control.Write(message)
	return err
}

func EncodeScrcpyTouch(action int, pointerID uint32, x, y, width, height int) ([]byte, error) {
	if action < 0 || action > 2 || width < 1 || height < 1 || width > 65535 || height > 65535 || x < 0 || y < 0 || x >= width || y >= height { return nil, fmt.Errorf("scrcpy touch coordinates are invalid") }
	message := make([]byte, 32)
	message[0], message[1] = 2, byte(action)
	binary.BigEndian.PutUint64(message[2:10], ^uint64(1)-uint64(pointerID))
	binary.BigEndian.PutUint32(message[10:14], uint32(x)); binary.BigEndian.PutUint32(message[14:18], uint32(y))
	binary.BigEndian.PutUint16(message[18:20], uint16(width)); binary.BigEndian.PutUint16(message[20:22], uint16(height))
	pressure := uint16(65535); if action == 1 { pressure = 0 }; binary.BigEndian.PutUint16(message[22:24], pressure)
	return message, nil
}

func (s *ScrcpySession) ReadPacket() ([]byte, bool, bool, error) {
	metadata := make([]byte, 12)
	if err := readExactly(s.video, metadata); err != nil { return nil, false, false, err }
	flags := binary.BigEndian.Uint64(metadata[:8])
	if flags&(1<<63) != 0 { s.width, s.height = int(binary.BigEndian.Uint32(metadata[4:8])), int(binary.BigEndian.Uint32(metadata[8:12])); return nil, false, false, nil }
	size := int(binary.BigEndian.Uint32(metadata[8:12])); if size < 1 || size > 16*1024*1024 { return nil, false, false, fmt.Errorf("scrcpy packet size is invalid") }
	packet := make([]byte, size); if err := readExactly(s.video, packet); err != nil { return nil, false, false, err }
	return packet, flags&(1<<62) != 0, flags&(1<<61) != 0, nil
}

func readExactly(reader io.Reader, buffer []byte) error { _, err := io.ReadFull(reader, buffer); return err }

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

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

const (
	defaultScrcpyMaxSize = 1280
	defaultScrcpyBitRate = 4_000_000
	defaultScrcpyFPS     = 30
	maxScrcpyPacketSize  = 16 * 1024 * 1024
)

type ScrcpyOptions struct {
	MaxSize   int
	BitRate   int
	FrameRate int
}

// ScrcpySession owns both forwarded sockets and the long-running Core child.
// Closing it never disconnects ADB; it only removes its own tcp forward and
// asks Core to stop the matching scrcpy app_process command.
type ScrcpySession struct {
	service   *Service
	deviceID  string
	sessionID string
	port      int
	video     net.Conn
	control   net.Conn
	controlMu sync.Mutex
	closeOnce sync.Once
	width     int
	height    int
}

type scrcpyStartResult struct {
	SessionID string `json:"sessionId"`
	ProcessID uint32 `json:"processId"`
}

func (s *Service) StartScrcpy(ctx context.Context, deviceID string, options ScrcpyOptions) (*ScrcpySession, error) {
	options = normalizeScrcpyOptions(options)
	apkPath, err := s.companionAPKPath(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	scid, err := randomScrcpyID()
	if err != nil {
		return nil, apperror.Wrap("SCRCPY_SESSION_ID_FAILED", "无法创建投屏会话", "device.screen", true, err)
	}
	sessionID := fmt.Sprintf("web-%08x", scid)
	socketName := fmt.Sprintf("scrcpy_%08x", scid)

	forward, err := s.Exec(ctx, DeviceArgs(deviceID, "forward", "tcp:0", "localabstract:"+socketName))
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(forward.Stdout))
	if err != nil || port < 1 || port > 65535 {
		return nil, apperror.Wrap("SCRCPY_FORWARD_INVALID", "ADB 未返回有效的投屏转发端口", "device.screen", true, err)
	}

	cleanupForward := func() {
		_, _ = s.Exec(context.Background(), DeviceArgs(deviceID, "forward", "--remove", fmt.Sprintf("tcp:%d", port)))
	}
	var started scrcpyStartResult
	err = s.core.Call(ctx, "adb.scrcpy.start", map[string]any{
		"sessionId":    sessionID,
		"deviceId":     deviceID,
		"apkPath":      apkPath,
		"scid":         scid,
		"maxSize":      options.MaxSize,
		"videoBitRate": options.BitRate,
		"maxFps":       options.FrameRate,
	}, &started)
	if err != nil {
		cleanupForward()
		return nil, err
	}
	cleanupProcess := func() {
		var ignored map[string]any
		_ = s.core.Call(context.Background(), "adb.scrcpy.stop", map[string]any{"sessionId": sessionID}, &ignored)
	}

	video, err := connectScrcpyVideo(ctx, port)
	if err != nil {
		cleanupProcess()
		cleanupForward()
		return nil, err
	}
	control, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		_ = video.Close()
		cleanupProcess()
		cleanupForward()
		return nil, apperror.Wrap("SCRCPY_CONTROL_UNAVAILABLE", "无法连接投屏触控通道", "device.screen", true, err)
	}
	width, height, err := readScrcpyHandshake(video)
	if err != nil {
		_ = video.Close()
		_ = control.Close()
		cleanupProcess()
		cleanupForward()
		return nil, err
	}

	return &ScrcpySession{
		service: s, deviceID: deviceID, sessionID: sessionID, port: port,
		video: video, control: control, width: width, height: height,
	}, nil
}

func normalizeScrcpyOptions(options ScrcpyOptions) ScrcpyOptions {
	if options.MaxSize < 128 || options.MaxSize > 4096 {
		options.MaxSize = defaultScrcpyMaxSize
	}
	if options.BitRate < 500_000 || options.BitRate > 32_000_000 {
		options.BitRate = defaultScrcpyBitRate
	}
	if options.FrameRate < 1 || options.FrameRate > 60 {
		options.FrameRate = defaultScrcpyFPS
	}
	return options
}

func (s *Service) companionAPKPath(ctx context.Context, deviceID string) (string, error) {
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "path", companionPackageName))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(output.Stdout, "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok && path != "" {
			return path, nil
		}
	}
	return "", apperror.New("SCRCPY_COMPANION_MISSING", "设备未安装 ADBControl 伴侣", "device.screen", true).
		WithSuggestion("请先在“ADB 伴侣”页面完成安装，再启动实时投屏")
}

func randomScrcpyID() (uint32, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(random[:]) | 0x10000000, nil
}

func connectScrcpyVideo(ctx context.Context, port int) (net.Conn, error) {
	deadline := time.Now().Add(8 * time.Second)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		connection, err := net.DialTimeout("tcp", address, 600*time.Millisecond)
		if err == nil {
			var dummy [1]byte
			_ = connection.SetReadDeadline(time.Now().Add(time.Second))
			_, readErr := io.ReadFull(connection, dummy[:])
			_ = connection.SetReadDeadline(time.Time{})
			if readErr == nil && dummy[0] == 0 {
				return connection, nil
			}
			lastErr = readErr
			_ = connection.Close()
		} else {
			lastErr = err
		}
		time.Sleep(80 * time.Millisecond)
	}
	return nil, apperror.Wrap("SCRCPY_VIDEO_UNAVAILABLE", "8 秒内未连接到投屏视频通道", "device.screen", true, lastErr)
}

func readScrcpyHandshake(connection net.Conn) (int, int, error) {
	codec := make([]byte, 4)
	if err := readExactly(connection, codec); err != nil {
		return 0, 0, apperror.Wrap("SCRCPY_HANDSHAKE_INVALID", "投屏编码握手不完整", "device.screen", true, err)
	}
	if string(codec) != "h264" {
		return 0, 0, apperror.New("SCRCPY_CODEC_UNSUPPORTED", fmt.Sprintf("投屏返回了不支持的编码 %q", string(codec)), "device.screen", false)
	}
	metadata := make([]byte, 12)
	if err := readExactly(connection, metadata); err != nil {
		return 0, 0, apperror.Wrap("SCRCPY_METADATA_INVALID", "投屏尺寸元数据不完整", "device.screen", true, err)
	}
	if metadata[0]&0x80 == 0 {
		return 0, 0, apperror.New("SCRCPY_METADATA_INVALID", "投屏缺少初始尺寸元数据", "device.screen", true)
	}
	width := int(binary.BigEndian.Uint32(metadata[4:8]))
	height := int(binary.BigEndian.Uint32(metadata[8:12]))
	if width < 1 || height < 1 || width > 65535 || height > 65535 {
		return 0, 0, apperror.New("SCRCPY_FRAME_SIZE_INVALID", "投屏返回了无效的画面尺寸", "device.screen", true)
	}
	return width, height, nil
}

func (s *ScrcpySession) Size() (int, int) { return s.width, s.height }

func (s *ScrcpySession) Close() {
	s.closeOnce.Do(func() {
		if s.video != nil {
			_ = s.video.Close()
		}
		if s.control != nil {
			_ = s.control.Close()
		}
		var ignored map[string]any
		_ = s.service.core.Call(context.Background(), "adb.scrcpy.stop", map[string]any{"sessionId": s.sessionID}, &ignored)
		_, _ = s.service.Exec(context.Background(), DeviceArgs(s.deviceID, "forward", "--remove", fmt.Sprintf("tcp:%d", s.port)))
	})
}

func (s *ScrcpySession) SendTouch(action int, pointerID uint32, x, y, width, height int) error {
	message, err := EncodeScrcpyTouch(action, pointerID, x, y, width, height)
	if err != nil {
		return err
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if _, err := s.control.Write(message); err != nil {
		return apperror.Wrap("SCRCPY_TOUCH_FAILED", "发送触控指令失败", "device.screen", true, err)
	}
	return nil
}

func EncodeScrcpyTouch(action int, pointerID uint32, x, y, width, height int) ([]byte, error) {
	if action < 0 || action > 2 || width < 1 || height < 1 || width > 65535 || height > 65535 || x < 0 || y < 0 || x >= width || y >= height {
		return nil, apperror.New("SCRCPY_TOUCH_INVALID", "投屏触控坐标无效", "device.screen", false)
	}
	message := make([]byte, 32)
	message[0], message[1] = 2, byte(action)
	binary.BigEndian.PutUint64(message[2:10], ^uint64(1)-uint64(pointerID))
	binary.BigEndian.PutUint32(message[10:14], uint32(x))
	binary.BigEndian.PutUint32(message[14:18], uint32(y))
	binary.BigEndian.PutUint16(message[18:20], uint16(width))
	binary.BigEndian.PutUint16(message[20:22], uint16(height))
	pressure := uint16(65535)
	if action == 1 {
		pressure = 0
	}
	binary.BigEndian.PutUint16(message[22:24], pressure)
	return message, nil
}

func (s *ScrcpySession) ReadPacket() ([]byte, bool, bool, error) {
	metadata := make([]byte, 12)
	if err := readExactly(s.video, metadata); err != nil {
		return nil, false, false, err
	}
	flags := binary.BigEndian.Uint64(metadata[:8])
	if flags&(1<<63) != 0 {
		s.width = int(binary.BigEndian.Uint32(metadata[4:8]))
		s.height = int(binary.BigEndian.Uint32(metadata[8:12]))
		return nil, false, false, nil
	}
	size := int(binary.BigEndian.Uint32(metadata[8:12]))
	if size < 1 || size > maxScrcpyPacketSize {
		return nil, false, false, apperror.New("SCRCPY_PACKET_INVALID", "投屏视频包大小无效", "device.screen", true)
	}
	packet := make([]byte, size)
	if err := readExactly(s.video, packet); err != nil {
		return nil, false, false, err
	}
	return packet, flags&(1<<62) != 0, flags&(1<<61) != 0, nil
}

func readExactly(reader io.Reader, buffer []byte) error {
	_, err := io.ReadFull(reader, buffer)
	return err
}

package coreipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ProcessTransport serializes requests because the original Core stdio protocol
// is ordered JSON Lines. The mutex is a protocol invariant, not a performance shortcut.
type ProcessTransport struct {
	mu      chan struct{}
	command *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
}

func StartProcess(ctx context.Context, binary string) (*ProcessTransport, error) {
	command := exec.CommandContext(ctx, binary)
	// Core stderr is diagnostic-only; forwarding it keeps IPC stdout strictly JSON Lines while making crashes observable.
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return &ProcessTransport{mu: make(chan struct{}, 1), command: command, stdin: stdin, scanner: scanner}, nil
}

func (t *ProcessTransport) RoundTrip(ctx context.Context, request Request) (Response, error) {
	if err := t.lock(ctx); err != nil {
		return Response{}, err
	}
	defer t.unlock()

	select {
	case <-ctx.Done():
		return Response{}, ctx.Err()
	default:
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	if _, err := t.stdin.Write(append(encoded, '\n')); err != nil {
		return Response{}, err
	}
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, fmt.Errorf("core process closed stdout")
	}
	var response Response
	if err := json.Unmarshal(t.scanner.Bytes(), &response); err != nil {
		return Response{}, err
	}
	return response, nil
}

func (t *ProcessTransport) Close() error {
	_ = t.lock(context.Background())
	defer t.unlock()
	_ = t.stdin.Close()
	if t.command.Process != nil {
		_ = t.command.Process.Kill()
	}
	return t.command.Wait()
}

func (t *ProcessTransport) lock(ctx context.Context) error {
	select {
	case t.mu <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *ProcessTransport) unlock() {
	select {
	case <-t.mu:
	default:
	}
}

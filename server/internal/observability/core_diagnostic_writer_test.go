package observability

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCoreDiagnosticWriterImportsSplitMobileLine(t *testing.T) {
	repository, err := OpenRepository(filepath.Join(t.TempDir(), "logs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := NewService(repository)
	writer := NewCoreDiagnosticWriter(service, nil)
	line := []byte(`{"event":"mobile_client_diagnostic","actor":"operator","sessionId":"s1","level":"error","phase":"quic.request","module":"remote.quic","ok":false,"elapsedMs":900,"errorCode":"REMOTE_QUIC_REQUEST_FAILED","detail":"timeout"}` + "\n")
	_, _ = writer.Write(line[:31])
	_, _ = writer.Write(line[31:])
	events, err := service.List(context.Background(), Query{Module: "mobile.remote.quic"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ErrorCode != "REMOTE_QUIC_REQUEST_FAILED" || events[0].Actor != "operator" {
		t.Fatalf("mobile diagnostic not imported: %#v", events)
	}
}

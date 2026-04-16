package listener

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAncoraListenerAuthenticateNoSecretAllowsConnection(t *testing.T) {
	t.Parallel()

	l := NewAncoraListener("/tmp/unused.sock", filepath.Join(t.TempDir(), "missing-secret"))
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	br := bufio.NewReader(client)
	if err := l.authenticate(client, br); err != nil {
		t.Fatalf("authenticate() error = %v, want nil", err)
	}
}

func TestAncoraListenerAuthenticateSendsSecretAndAcceptsOK(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "ipc-secret")
	if err := osWriteFile(secretPath, []byte("abc123\n")); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	l := NewAncoraListener("/tmp/unused.sock", secretPath)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(server).ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if got, want := line, "AUTH abc123\n"; got != want {
			done <- testErrf("auth line = %q, want %q", got, want)
			return
		}
		_, err = server.Write([]byte("OK\n"))
		done <- err
	}()

	br := bufio.NewReader(client)
	if err := l.authenticate(client, br); err != nil {
		t.Fatalf("authenticate() error = %v, want nil", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server goroutine error = %v", err)
	}
}

func TestAncoraListenerAuthenticateRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "ipc-secret")
	if err := osWriteFile(secretPath, []byte("wrong-secret")); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	l := NewAncoraListener("/tmp/unused.sock", secretPath)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = server.Write([]byte("ERR unauthorized\n"))
	}()

	br := bufio.NewReader(client)
	err := l.authenticate(client, br)
	if err == nil {
		t.Fatal("authenticate() error = nil, want auth rejection")
	}
	if !strings.Contains(err.Error(), "auth rejected") {
		t.Fatalf("authenticate() error = %v, want auth rejected", err)
	}
}

func TestAncoraListenerStreamDispatchesEvents(t *testing.T) {
	t.Parallel()

	l := NewAncoraListener("/tmp/unused.sock", "")
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	br := bufio.NewReaderSize(client, 1<<20)
	go l.stream(ctx, client, br)

	payload := json.RawMessage(`{"id":123}`)
	frame, err := json.Marshal(wireEvent{
		Type:      EventObservationCreated,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal wire event: %v", err)
	}

	if _, err := server.Write(append(frame, '\n')); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	select {
	case ev := <-l.Events():
		if ev.Source != "ancora" {
			t.Fatalf("event source = %q, want ancora", ev.Source)
		}
		if ev.Type != EventObservationCreated {
			t.Fatalf("event type = %q, want %q", ev.Type, EventObservationCreated)
		}
		if string(ev.Payload) != string(payload) {
			t.Fatalf("event payload = %s, want %s", ev.Payload, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for streamed event")
	}

	status := l.Status()
	if status.EventCount != 1 {
		t.Fatalf("status.EventCount = %d, want 1", status.EventCount)
	}
	if status.LastEvent.IsZero() {
		t.Fatal("status.LastEvent = zero, want populated timestamp")
	}
}

// TestAncoraListenerSharedReaderNoByteLoss verifies that when the server
// writes OK\n and an event frame in a single write (as the IPC server does),
// the event is NOT lost because auth and stream share the same bufio.Reader.
func TestAncoraListenerSharedReaderNoByteLoss(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "ipc-secret")
	if err := osWriteFile(secretPath, []byte("s3cr3t\n")); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	l := NewAncoraListener("/tmp/unused.sock", secretPath)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := json.RawMessage(`{"id":99}`)
	frame, _ := json.Marshal(wireEvent{
		Type:      EventObservationCreated,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})

	// Server: read AUTH line then send OK + event in ONE write (same TCP segment).
	go func() {
		_, _ = bufio.NewReader(server).ReadString('\n')
		combined := append([]byte("OK\n"), append(frame, '\n')...)
		_, _ = server.Write(combined)
	}()

	// Client: authenticate then stream — both using same bufio.Reader.
	br := bufio.NewReaderSize(client, 1<<20)
	if err := l.authenticate(client, br); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go l.stream(ctx, client, br)

	select {
	case ev := <-l.Events():
		if ev.Type != EventObservationCreated {
			t.Fatalf("event type = %q, want %q", ev.Type, EventObservationCreated)
		}
	case <-ctx.Done():
		t.Fatal("timed out — shared-reader bug: event lost in auth buffer")
	}
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

type testErr string

func (e testErr) Error() string { return string(e) }

func testErrf(format string, args ...any) error {
	return testErr(fmt.Sprintf(format, args...))
}

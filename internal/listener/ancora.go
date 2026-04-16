package listener

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// wireEvent is the NDJSON frame sent over the IPC socket.
type wireEvent struct {
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// AncoraListener connects to the Ancora IPC socket and relays events to Vela's
// reconciler. It implements automatic reconnection with exponential backoff.
type AncoraListener struct {
	socketPath string // e.g. ~/.syfra/ancora.sock
	secretPath string // e.g. ~/.syfra/ipc-secret
	events     chan Event
	closeCh    chan struct{}
	closeOnce  sync.Once

	mu         sync.RWMutex
	connected  bool
	lastEvent  time.Time
	lastErr    error
	eventCount int64 // atomic

	reconnectBase time.Duration
	reconnectMax  time.Duration
}

const (
	defaultSocketDir   = ".syfra"
	ancoraSocketName   = "ancora.sock"
	ipcSecretName      = "ipc-secret"
	eventChannelBuffer = 256
)

// NewAncoraListener creates a listener for the Ancora IPC socket.
// socketPath and secretPath may be empty — defaults under ~/.syfra/ are used.
func NewAncoraListener(socketPath, secretPath string) *AncoraListener {
	if socketPath == "" {
		home, _ := os.UserHomeDir()
		socketPath = filepath.Join(home, defaultSocketDir, ancoraSocketName)
	}
	if secretPath == "" {
		home, _ := os.UserHomeDir()
		secretPath = filepath.Join(home, defaultSocketDir, ipcSecretName)
	}
	return &AncoraListener{
		socketPath:    socketPath,
		secretPath:    secretPath,
		events:        make(chan Event, eventChannelBuffer),
		closeCh:       make(chan struct{}),
		reconnectBase: 2 * time.Second,
		reconnectMax:  60 * time.Second,
	}
}

// Name implements EventSource.
func (a *AncoraListener) Name() string { return "ancora" }

// Connect starts the background reader goroutine. Returns immediately.
// If the socket is unavailable the goroutine will retry automatically.
func (a *AncoraListener) Connect(ctx context.Context) error {
	go a.readLoop(ctx)
	return nil
}

// Events implements EventSource.
func (a *AncoraListener) Events() <-chan Event { return a.events }

// Close implements EventSource.
func (a *AncoraListener) Close() error {
	a.closeOnce.Do(func() {
		close(a.closeCh)
	})
	return nil
}

// Status implements EventSource.
func (a *AncoraListener) Status() SourceStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return SourceStatus{
		Connected:  a.connected,
		LastEvent:  a.lastEvent,
		EventCount: atomic.LoadInt64(&a.eventCount),
		LastError:  a.lastErr,
	}
}

// readLoop connects to the socket, authenticates, and streams events.
// On disconnect it backs off and retries until Close is called.
func (a *AncoraListener) readLoop(ctx context.Context) {
	backoff := a.reconnectBase
	for {
		select {
		case <-a.closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		conn, err := a.dial()
		if err != nil {
			a.setError(err)
			select {
			case <-a.closeCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, a.reconnectMax)
				continue
			}
		}

		backoff = a.reconnectBase // reset on success
		a.setConnected(true)

		// Wrap conn in a single shared reader so auth + stream share the
		// same buffer. Two separate bufio.Scanner on the same net.Conn
		// would cause the auth scanner to buffer (and discard) early event
		// bytes before the stream scanner is created.
		br := bufio.NewReaderSize(conn, 1<<20)

		if err := a.authenticate(conn, br); err != nil {
			a.setError(fmt.Errorf("auth: %w", err))
			conn.Close()
			a.setConnected(false)
			log.Printf("[ancora] listener: auth failed: %v", err)
			continue
		}

		log.Printf("[ancora] listener: connected to %s", a.socketPath)
		a.stream(ctx, conn, br)
		conn.Close()
		a.setConnected(false)
		log.Printf("[ancora] listener: disconnected from %s (will reconnect)", a.socketPath)
	}
}

func (a *AncoraListener) dial() (net.Conn, error) {
	return net.DialTimeout("unix", a.socketPath, 5*time.Second)
}

// authenticate sends the shared secret and waits for OK.
// br is the shared buffered reader wrapping conn — it must be reused in
// stream() so that any bytes buffered during auth are not lost.
func (a *AncoraListener) authenticate(conn net.Conn, br *bufio.Reader) error {
	secret, err := os.ReadFile(a.secretPath)
	if err != nil {
		// No secret file — proceed unauthenticated (local dev / no-auth mode)
		return nil
	}

	hexSecret := string(secret)
	if len(hexSecret) > 0 && hexSecret[len(hexSecret)-1] == '\n' {
		hexSecret = hexSecret[:len(hexSecret)-1]
	}

	if _, err := fmt.Fprintf(conn, "AUTH %s\n", hexSecret); err != nil {
		return fmt.Errorf("sending auth: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := br.ReadString('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("no auth response: %w", err)
	}
	// Trim trailing newline/CR before comparing.
	resp = strings.TrimRight(resp, "\r\n")

	if resp != "OK" {
		return fmt.Errorf("auth rejected: %s", resp)
	}
	return nil
}

// stream reads NDJSON frames from br and dispatches them to the events channel.
// br is the shared bufio.Reader created before authenticate() so no bytes are lost.
func (a *AncoraListener) stream(ctx context.Context, conn net.Conn, br *bufio.Reader) {
	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB max line

	for scanner.Scan() {
		select {
		case <-a.closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		var wire wireEvent
		if err := json.Unmarshal(scanner.Bytes(), &wire); err != nil {
			a.setError(fmt.Errorf("parse: %w", err))
			continue
		}

		ev := Event{
			Source:    a.Name(),
			Type:      wire.Type,
			Payload:   wire.Payload,
			Timestamp: wire.Timestamp,
		}

		select {
		case a.events <- ev:
			atomic.AddInt64(&a.eventCount, 1)
			a.mu.Lock()
			a.lastEvent = time.Now()
			a.mu.Unlock()
		default:
			// Channel full — drop oldest by draining one and re-trying
			select {
			case <-a.events:
			default:
			}
			a.events <- ev
		}
	}

	if err := scanner.Err(); err != nil {
		a.setError(err)
	}
}

func (a *AncoraListener) setConnected(v bool) {
	a.mu.Lock()
	a.connected = v
	a.mu.Unlock()
}

func (a *AncoraListener) setError(err error) {
	a.mu.Lock()
	a.lastErr = err
	a.connected = false
	a.mu.Unlock()
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

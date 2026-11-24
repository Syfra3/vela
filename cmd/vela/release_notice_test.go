package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type releaseRoundTripFunc func(*http.Request) (*http.Response, error)

func (f releaseRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func runReleaseCommand(t *testing.T, ctx context.Context, installed string, transport releaseRoundTripFunc, args ...string) (string, string) {
	t.Helper()
	previous, previousUnstamped := version, unstampedBuild
	version, unstampedBuild = installed, false
	t.Cleanup(func() { version, unstampedBuild = previous, previousUnstamped })
	return executeReleaseCommand(t, ctx, transport, args...)
}

func executeReleaseCommand(t *testing.T, ctx context.Context, transport releaseRoundTripFunc, args ...string) (string, string) {
	t.Helper()
	root := rootCmdWithReleaseTransport(transport)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return stdout.String(), stderr.String()
}

func TestVersionReleaseUnstampedBuild(t *testing.T) {
	stdout, stderr := executeReleaseCommand(t, context.Background(), func(req *http.Request) (*http.Response, error) {
		t.Fatal("unstamped build unexpectedly checked the latest release")
		return nil, nil
	}, "version")
	if stdout != "vela "+Version+"\n" || stderr != "" {
		t.Fatalf("stdout=%q; stderr=%q", stdout, stderr)
	}
}

func TestVersionReleaseNotice(t *testing.T) {
	for _, tc := range []struct {
		name, installed, body string
		status, requests      int
		optOut, notice        bool
	}{
		{name: "newer", body: `{"tag_name":"v1.10.0"}`, status: 200, requests: 1, notice: true},
		{name: "stamped at compiled default", installed: Version, body: `{"tag_name":"v` + Version + `"}`, status: 200, requests: 1},
		{name: "equal with metadata", body: `{"tag_name":"v1.9.0+build"}`, status: 200, requests: 1},
		{name: "older", body: `{"tag_name":"v1.8.99"}`, status: 200, requests: 1},
		{name: "malformed tag", body: `{"tag_name":"v01.99.0"}`, status: 200, requests: 1},
		{name: "prerelease tag", body: `{"tag_name":"v2.0.0-rc.1"}`, status: 200, requests: 1},
		{name: "draft", body: `{"tag_name":"v2.0.0","draft":true}`, status: 200, requests: 1},
		{name: "prerelease flag", body: `{"tag_name":"v2.0.0","prerelease":true}`, status: 200, requests: 1},
		{name: "invalid JSON", body: `{"tag_name":"v2.0.0"}garbage`, status: 200, requests: 1},
		{name: "HTTP failure", body: `{"tag_name":"v2.0.0"}`, status: 503, requests: 1},
		{name: "redirect", status: 302, requests: 1},
		{name: "transport failure", requests: 1},
		{name: "opt out", optOut: true},
		{name: "dev", installed: "dev"},
		{name: "installed prerelease", installed: "2.0.0-rc.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installed := tc.installed
			if installed == "" {
				installed = "1.9.0"
			}
			args := []string{"version"}
			if tc.optOut {
				args = append(args, "--no-update-check")
			}
			requests := 0
			stdout, stderr := runReleaseCommand(t, context.Background(), installed, func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != http.MethodGet || req.URL.String() != "https://api.github.com/repos/Syfra3/vela/releases/latest" || req.Header.Get("Authorization") != "" {
					t.Errorf("unexpected request: %s %s", req.Method, req.URL)
				}
				if tc.status == 0 {
					return nil, errors.New("offline transport failure")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": {"https://example.invalid/"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}, args...)
			if stdout != "vela "+installed+"\n" || requests != tc.requests {
				t.Errorf("stdout=%q; requests=%d, want %d", stdout, requests, tc.requests)
			}
			if tc.notice {
				for _, part := range []string{"v1.10.0", "brew update && brew upgrade vela", "https://github.com/Syfra3/vela/releases"} {
					if !strings.Contains(stderr, part) {
						t.Errorf("stderr %q missing %q", stderr, part)
					}
				}
			} else if stderr != "" {
				t.Errorf("unexpected stderr: %q", stderr)
			}
		})
	}
}

func TestVersionReleaseDeadline(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		name := "timeout"
		if cancelParent {
			name = "parent cancellation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := time.Now()
			var got error
			stdout, stderr := runReleaseCommand(t, ctx, "1.9.0", func(req *http.Request) (*http.Response, error) {
				deadline, ok := req.Context().Deadline()
				if !ok || deadline.After(started.Add(time.Second+100*time.Millisecond)) || time.Until(deadline) > time.Second {
					t.Fatalf("unexpected request deadline: %v", deadline)
				}
				if cancelParent {
					cancel()
				}
				<-req.Context().Done()
				got = req.Context().Err()
				return nil, got
			}, "version")
			want := context.DeadlineExceeded
			if cancelParent {
				want = context.Canceled
			}
			if got != want || stdout != "vela 1.9.0\n" || stderr != "" {
				t.Fatalf("request error=%v, want %v; stdout=%q; stderr=%q", got, want, stdout, stderr)
			}
		})
	}
}

func TestVersionReleaseOtherCommand(t *testing.T) {
	stdout, stderr := runReleaseCommand(t, context.Background(), "1.9.0", func(req *http.Request) (*http.Response, error) {
		t.Fatal("root help unexpectedly checked the latest release")
		return nil, nil
	}, "help")
	if !strings.Contains(stdout, "Available Commands:") || stderr != "" {
		t.Fatalf("stdout=%q; stderr=%q", stdout, stderr)
	}
}

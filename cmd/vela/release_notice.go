package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	latestReleaseURL = "https://api.github.com/repos/Syfra3/vela/releases/latest"
	releasesURL      = "https://github.com/Syfra3/vela/releases"
	releaseTimeout   = time.Second
	maxReleaseBody   = 1 << 20
)

var stableVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func newerRelease(ctx context.Context, transport http.RoundTripper, installed string) string {
	current := stableVersionPattern.FindStringSubmatch(installed)
	if current == nil || ctx.Err() != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return ""
	}
	client := http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxReleaseBody+1))
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err != nil || len(body) > maxReleaseBody || json.Unmarshal(body, &release) != nil || release.Draft || release.Prerelease || ctx.Err() != nil {
		return ""
	}
	latest := stableVersionPattern.FindStringSubmatch(release.Tag)
	if latest == nil {
		return ""
	}
	for i := 1; i <= 3; i++ {
		// Compare canonical decimal components without integer overflow.
		cmp := len(latest[i]) - len(current[i])
		if cmp == 0 {
			cmp = strings.Compare(latest[i], current[i])
		}
		if cmp > 0 {
			return release.Tag
		}
		if cmp < 0 {
			break
		}
	}
	return ""
}

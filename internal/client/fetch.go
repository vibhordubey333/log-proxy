package client

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
)

// FetchLog GETs the log for buildID from the proxy at proxyURL and
// returns it split into lines. Splitting here (rather than returning
// a raw string) keeps the dedup package's line-based API consistent
// regardless of where the lines came from.
func FetchLog(proxyURL, buildID string) ([]string, error) {
	url := fmt.Sprintf("%s/logs/%s", proxyURL, buildID)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching from proxy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("proxy returned %d: %s", resp.StatusCode, body)
	}

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	// Jenkins logs can have long lines (e.g. huge dependency trees);
	// bufio.Scanner's default 64KB max line length isn't always enough.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return lines, nil
}

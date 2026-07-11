package proxy

import (
	"context"
	"fmt"
	"io"
	"os"
)

// FixtureFetcher serves a single local file as the "log" for every
// build ID, regardless of what's requested. Used for local manual
// testing (make run-server-fixture) since ci.jenkins.io now requires auth

type FixtureFetcher struct {
	FilePath    string
	ContentType string
}

func (f *FixtureFetcher) Fetch(ctx context.Context, buildID string) (io.ReadCloser, string, error) {
	file, err := os.Open(f.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening fixture file: %w", err)
	}
	return file, f.ContentType, nil
}

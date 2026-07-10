package proxy

import (
	"context"
	"fmt"
	"golang.org/x/sync/singleflight"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var (
	buildIDPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
)

type CachedLog struct {
	Path        string
	ContentType string
}

// Cache resolves a build ID to a cached log file on disk, downloading  it via fetcher.go if it isn't already cached. Concurrent
// requests for the same uncached build ID are collapsed into a single download via singleflight.
type Cache struct {
	Dir     string
	Fetcher LogFetcher
	group   singleflight.Group
}

func (c *Cache) Resolve(ctx context.Context, buildID string) (*CachedLog, error) {
	if !buildIDPattern.MatchString(buildID) {
		return nil, ErrInvalidBuildID
	}

	// Store the downloaded log and its HTTP metadata separately: the .log file
	// contains the response body, while the .meta file records the content type
	// needed when serving the cached log later.
	logPath := filepath.Join(c.Dir, buildID+".log")
	metaPath := filepath.Join(c.Dir, buildID+".meta")

	// Fast path: already cached, no need to involve singleflight at all.
	if _, err := os.Stat(logPath); err == nil {
		ct, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, fmt.Errorf("reading cached content-type: %w", err)
		}
		return &CachedLog{Path: logPath, ContentType: string(ct)}, nil
	}

	// Slowq path: not cached yet. singleflight.Do ensures that if ten requests for the same buildID arrive at once, only one of them
	// actually executes this function; the other nine block and receive the same result.
	result, err, _ := c.group.Do(buildID, func() (interface{}, error) {
		return c.download(ctx, buildID, logPath, metaPath)
	})
	if err != nil {
		return nil, err
	}
	return result.(*CachedLog), nil
}

func (c *Cache) download(ctx context.Context, buildID, logPath, metaPath string) (*CachedLog, error) {
	// Re-check on entry: another goroutine may have raced us before
	// acquiring the singleflight key (e.g. two separate Resolve calls
	// arriving microseconds apart, before either registered with the group).
	if _, err := os.Stat(logPath); err == nil {
		ct, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, fmt.Errorf("reading cached content-type: %w", err)
		}
		return &CachedLog{Path: logPath, ContentType: string(ct)}, nil
	}

	body, contentType, err := c.Fetcher.Fetch(ctx, buildID)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	tmpPath := logPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("creating temp cache file: %w", err)
	}

	if _, err := io.Copy(tmpFile, body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("writing cache file: %w", err)
	}
	tmpFile.Close()

	// Write completed successfully, so publish the cache file by renaming the
	// temporary file into place. On the same filesystem, rename is atomic: readers
	// will either see no .log file or the complete .log file, never a partially
	// written one.
	if err := os.Rename(tmpPath, logPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("finalizing cache file: %w", err)
	}

	if err := os.WriteFile(metaPath, []byte(contentType), 0644); err != nil {
		return nil, fmt.Errorf("writing content-type metadata: %w", err)
	}

	return &CachedLog{Path: logPath, ContentType: contentType}, nil
}

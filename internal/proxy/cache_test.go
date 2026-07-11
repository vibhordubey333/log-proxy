// Package proxy_test-equivalent coverage for the caching layer.
//
// These tests exercise Cache.Resolve against fakeFetcher, a LogFetcher
// test double, rather than any real HTTP server or real Jenkins. This
// keeps the tests fast, deterministic, and independent of network
// access -- see REFLECTION.md for why real Jenkins access isn't used
// anywhere in this codebase's test suite.
package proxy

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFetcher is a test double implementing LogFetcher. It counts how
// many times Fetch was actually called (not how many times Resolve was
// called), which is exactly what we need to verify singleflight
// behavior, and optionally sleeps and/or returns a canned error.
type fakeFetcher struct {
	content     string        // body to return on a successful Fetch
	contentType string        // content-type to return alongside content
	err         error         // if non-nil, Fetch returns this error instead of content
	sleep       time.Duration // artificial delay before returning, used to widen the window for concurrent calls to overlap
	calls       int32         // number of times Fetch has actually executed; read/written atomically since tests call this concurrently
}

// Fetch implements LogFetcher. It records a call, optionally sleeps
// (to simulate a slow upstream and give concurrent callers a chance to
// overlap), then returns either the canned error or the canned content.
func (f *fakeFetcher) Fetch(ctx context.Context, buildID string) (io.ReadCloser, string, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	if f.err != nil {
		return nil, "", f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), f.contentType, nil
}

// callCount returns the number of times Fetch has been called so far.
// Uses atomic.LoadInt32 rather than a plain field read since tests
// call this while goroutines may still be concurrently incrementing
// f.calls -- a plain read here would itself be a data race under
// `go test -race`.
func (f *fakeFetcher) callCount() int32 {
	return atomic.LoadInt32(&f.calls)
}

// newTestCache builds a Cache backed by an isolated, auto-cleaned
// temporary directory, so tests never share cache state with each
// other or leave files behind on disk after the test finishes.
func newTestCache(t *testing.T, fetcher LogFetcher) *Cache {
	t.Helper()
	dir := t.TempDir() // removed automatically by the testing package after the test completes
	return &Cache{Dir: dir, Fetcher: fetcher}
}

// TestCache_Resolve_DownloadsOnce verifies the basic happy path: a
// single Resolve call for a not-yet-cached build ID downloads the
// content exactly once, writes it to disk, and returns the correct
// content type.
func TestCache_Resolve_DownloadsOnce(t *testing.T) {
	fetcher := &fakeFetcher{content: "hello world", contentType: "text/plain"}
	cache := newTestCache(t, fetcher)

	got, err := cache.Resolve(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ContentType != "text/plain" {
		t.Errorf("content type = %q, want %q", got.ContentType, "text/plain")
	}

	data, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatalf("reading cached file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("cached content = %q, want %q", data, "hello world")
	}
	if fetcher.callCount() != 1 {
		t.Errorf("fetch called %d times, want 1", fetcher.callCount())
	}
}

// TestCache_Resolve_CacheHitSkipsFetch verifies that a second Resolve
// call for the same build ID hits the on-disk cache and does not call
// Fetch again. This is the core "remote file should only be downloaded
// once" requirement from the spec, in its simplest (non-concurrent)
// form.
func TestCache_Resolve_CacheHitSkipsFetch(t *testing.T) {
	fetcher := &fakeFetcher{content: "hello world", contentType: "text/plain"}
	cache := newTestCache(t, fetcher)

	if _, err := cache.Resolve(context.Background(), "123"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := cache.Resolve(context.Background(), "123"); err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if fetcher.callCount() != 1 {
		t.Errorf("fetch called %d times across two resolves for same id, want 1", fetcher.callCount())
	}
}

// TestCache_Resolve_ConcurrentRequestsCollapseIntoOneDownload is the
// most important test in this file: it verifies that when many
// goroutines request the same *uncached* build ID at the same time,
// only one of them actually triggers a download -- the rest wait and
// receive the same result. This is what singleflight buys us, and it's
// the concurrent case the spec's "should only be downloaded once"
// requirement is really guarding against (a single sequential request
// is the easy case; simultaneous cold requests are the interesting
// one).
//
// The fakeFetcher's artificial sleep widens the window during which
// all 10 goroutines are guaranteed to have called Resolve before the
// first one finishes downloading, so this test isn't relying on timing
// luck to actually exercise the race.
func TestCache_Resolve_ConcurrentRequestsCollapseIntoOneDownload(t *testing.T) {
	fetcher := &fakeFetcher{
		content:     "hello world",
		contentType: "text/plain",
		sleep:       100 * time.Millisecond, // wide enough for all goroutines to arrive concurrently
	}
	cache := newTestCache(t, fetcher)

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := cache.Resolve(context.Background(), "999")
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	if fetcher.callCount() != 1 {
		t.Errorf("fetch called %d times for %d concurrent requests on same id, want 1", fetcher.callCount(), n)
	}
}

// TestCache_Resolve_DifferentBuildIDsEachFetchOnce verifies that
// caching is correctly keyed per build ID -- singleflight and the
// on-disk cache must not accidentally collapse or conflate requests
// for genuinely different builds. Two distinct IDs should result in
// two distinct fetches, not one.
func TestCache_Resolve_DifferentBuildIDsEachFetchOnce(t *testing.T) {
	fetcher := &fakeFetcher{content: "hello world", contentType: "text/plain"}
	cache := newTestCache(t, fetcher)

	if _, err := cache.Resolve(context.Background(), "111"); err != nil {
		t.Fatalf("resolve 111: %v", err)
	}
	if _, err := cache.Resolve(context.Background(), "222"); err != nil {
		t.Fatalf("resolve 222: %v", err)
	}

	if fetcher.callCount() != 2 {
		t.Errorf("fetch called %d times for 2 distinct ids, want 2", fetcher.callCount())
	}
}

// TestCache_Resolve_InvalidBuildID verifies the path-traversal /
// input-validation guard: build IDs containing anything other than
// alphanumeric characters (including attempted traversal like
// "../etc/passwd") are rejected with ErrInvalidBuildID before Fetch is
// ever called -- untrusted input must never reach the filesystem or
// the upstream fetch.
func TestCache_Resolve_InvalidBuildID(t *testing.T) {
	fetcher := &fakeFetcher{content: "hello world"}
	cache := newTestCache(t, fetcher)

	cases := []string{"../etc/passwd", "has space", "build.1", "weird$id", ""}
	for _, id := range cases {
		_, err := cache.Resolve(context.Background(), id)
		if !errors.Is(err, ErrInvalidBuildID) {
			t.Errorf("build id %q: got err %v, want ErrInvalidBuildID", id, err)
		}
	}
	if fetcher.callCount() != 0 {
		t.Errorf("fetch should never be called for invalid ids, got %d calls", fetcher.callCount())
	}
}

// TestCache_Resolve_FetchErrorLeavesNoPartialFile verifies the
// atomic-write guarantee: if the upstream fetch itself fails, no file
// -- neither a finished .log nor a leftover .tmp -- should exist in
// the cache directory afterward. This matters because a half-written
// file left on disk could later be mistakenly served to a client as a
// valid cache hit.
func TestCache_Resolve_FetchErrorLeavesNoPartialFile(t *testing.T) {
	fetcher := &fakeFetcher{err: ErrUpstreamUnavailable}
	cache := newTestCache(t, fetcher)

	_, err := cache.Resolve(context.Background(), "555")
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("got err %v, want ErrUpstreamUnavailable", err)
	}

	// Neither the final .log nor a leftover .tmp should exist -- the
	// error happened inside Fetch, before any file was even created.
	entries, err := os.ReadDir(cache.Dir)
	if err != nil {
		t.Fatalf("reading cache dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty cache dir after fetch error, found %d entries", len(entries))
	}
}

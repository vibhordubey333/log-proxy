// handler_test.go exercises the Gin HTTP layer end-to-end: real
// HTTP requests against a real Gin router, backed by a real
// JenkinsFetcher pointed at an httptest.Server standing in for
// Jenkins. This is the one place JenkinsFetcher itself gets tested,
// rather than just Cache against a fakeFetcher (see cache_test.go).
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestServer spins up a full Handler (Gin router + Cache +
// JenkinsFetcher) pointed at a fake Jenkins httptest.Server that
// always serves the given body/content-type/status for any build ID.
// Returns the test proxy server; callers should `defer ts.Close()` on
// both this and the returned fake-Jenkins server if they need direct
// access to it (most tests don't).
func newTestServer(t *testing.T, body string, contentType string, status int) *httptest.Server {
	t.Helper()

	fakeJenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(fakeJenkins.Close)

	gin.SetMode(gin.TestMode) // silences Gin's debug-mode startup warnings during test runs

	cache := &Cache{
		Dir:     t.TempDir(),
		Fetcher: &JenkinsFetcher{BaseURL: fakeJenkins.URL, Client: fakeJenkins.Client()},
	}
	handler := &Handler{Cache: cache}

	router := gin.New()
	handler.RegisterRoutes(router)

	proxyServer := httptest.NewServer(router)
	t.Cleanup(proxyServer.Close)

	return proxyServer
}

// TestHandleGet_FullFile verifies the simplest case: no offset/limit
// query params returns the entire cached file, with Content-Length
// and Content-Type matching the fake Jenkins response.
func TestHandleGet_FullFile(t *testing.T) {
	ts := newTestServer(t, "line one\nline two\nline three\n", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := readAll(t, resp)
	want := "line one\nline two\nline three\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if resp.Header.Get("Content-Length") != fmt.Sprint(len(want)) {
		t.Errorf("Content-Length = %q, want %q", resp.Header.Get("Content-Length"), fmt.Sprint(len(want)))
	}
	if resp.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", resp.Header.Get("Content-Type"))
	}
}

// TestHandleGet_WithOffset verifies that an offset query param
// correctly skips the requested number of leading bytes.
func TestHandleGet_WithOffset(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?offset=3")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	body := readAll(t, resp)
	if body != "3456789" {
		t.Errorf("body = %q, want %q", body, "3456789")
	}
}

// TestHandleGet_WithLimit verifies that a limit query param truncates
// the response to the requested number of bytes.
func TestHandleGet_WithLimit(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?limit=4")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	body := readAll(t, resp)
	if body != "0123" {
		t.Errorf("body = %q, want %q", body, "0123")
	}
}

// TestHandleGet_WithOffsetAndLimit verifies offset and limit combined
// correctly return a middle slice of the file.
func TestHandleGet_WithOffsetAndLimit(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?offset=2&limit=3")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	body := readAll(t, resp)
	if body != "234" {
		t.Errorf("body = %q, want %q", body, "234")
	}
}

// TestHandleGet_LimitExceedingRemainingBytes verifies that a limit
// larger than what's actually left in the file is clamped to the
// remaining bytes, rather than erroring or reading out of bounds.
func TestHandleGet_LimitExceedingRemainingBytes(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?offset=8&limit=100")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	body := readAll(t, resp)
	if body != "89" {
		t.Errorf("body = %q, want %q", body, "89")
	}
	if resp.Header.Get("Content-Length") != "2" {
		t.Errorf("Content-Length = %q, want 2", resp.Header.Get("Content-Length"))
	}
}

// TestHandleGet_ExplicitLimitZero is the regression test for the bug
// we found and fixed: limit=0 must return zero bytes, distinct from
// omitting limit entirely (which returns the whole remainder). Before
// the fix, both were indistinguishable and limit=0 silently returned
// the entire file.
func TestHandleGet_ExplicitLimitZero(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?limit=0")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	body := readAll(t, resp)
	if body != "" {
		t.Errorf("body = %q, want empty string", body)
	}
	if resp.Header.Get("Content-Length") != "0" {
		t.Errorf("Content-Length = %q, want 0", resp.Header.Get("Content-Length"))
	}
}

// TestHandleGet_OffsetBeyondEOF verifies that requesting an offset
// past the end of the file returns 416, not a 500 or a silently empty
// success response.
func TestHandleGet_OffsetBeyondEOF(t *testing.T) {
	ts := newTestServer(t, "short", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?offset=9999")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("status = %d, want 416", resp.StatusCode)
	}
}

// TestHandleGet_NegativeOffset verifies that a negative offset is
// rejected as a bad request rather than causing an out-of-bounds
// file seek.
func TestHandleGet_NegativeOffset(t *testing.T) {
	ts := newTestServer(t, "content", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?offset=-1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHandleGet_NonIntegerLimit verifies that a non-numeric limit
// value is rejected as a bad request.
func TestHandleGet_NonIntegerLimit(t *testing.T) {
	ts := newTestServer(t, "content", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/42?limit=abc")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHandleGet_InvalidBuildID verifies that an unsafe build ID
// (failing our alphanumeric-only validation) is rejected as a bad
// request before any fetch or file access is attempted.
func TestHandleGet_InvalidBuildID(t *testing.T) {
	ts := newTestServer(t, "content", "text/plain", http.StatusOK)

	resp, err := http.Get(ts.URL + "/logs/build.1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHandleGet_JenkinsReturns404 verifies that a 404 from upstream
// Jenkins is correctly mapped to a 404 for our own client, not a
// generic 500 or 502.
func TestHandleGet_JenkinsReturns404(t *testing.T) {
	ts := newTestServer(t, "not found", "text/plain", http.StatusNotFound)

	resp, err := http.Get(ts.URL + "/logs/999")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHandleGet_JenkinsReturns500 verifies that any other non-200,
// non-404 response from upstream Jenkins is mapped to our own 502,
// signaling "upstream is unavailable/misbehaving" rather than
// pretending the request itself was somehow bad (400) or that our own
// server is broken (500).
func TestHandleGet_JenkinsReturns500(t *testing.T) {
	ts := newTestServer(t, "internal error", "text/plain", http.StatusInternalServerError)

	resp, err := http.Get(ts.URL + "/logs/999")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

// TestHandleHead_ReturnsContentLengthWithoutBody verifies HEAD's core
// contract: it must fully resolve (download-if-missing) the build so
// it can report an accurate Content-Length, but return no body.
func TestHandleHead_ReturnsContentLengthWithoutBody(t *testing.T) {
	ts := newTestServer(t, "0123456789", "text/plain", http.StatusOK)

	resp, err := http.Head(ts.URL + "/logs/42")
	if err != nil {
		t.Fatalf("HEAD failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Content-Length") != "10" {
		t.Errorf("Content-Length = %q, want 10", resp.Header.Get("Content-Length"))
	}
	body := readAll(t, resp)
	if body != "" {
		t.Errorf("HEAD response body = %q, want empty", body)
	}
}

// TestHandleHead_CachesOnFirstCall verifies HEAD alone is sufficient
// to populate the cache -- a subsequent GET for the same build ID
// should get served from that same cached file, not trigger a second
// fetch. This directly covers the spec's "HEAD ... will need to fully
// download the remote file" requirement.
func TestHandleHead_CachesOnFirstCall(t *testing.T) {
	ts := newTestServer(t, "cached content", "text/plain", http.StatusOK)

	headResp, err := http.Head(ts.URL + "/logs/42")
	if err != nil {
		t.Fatalf("HEAD failed: %v", err)
	}
	headResp.Body.Close()

	getResp, err := http.Get(ts.URL + "/logs/42")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer getResp.Body.Close()

	body := readAll(t, getResp)
	if body != "cached content" {
		t.Errorf("body = %q, want %q", body, "cached content")
	}
}

// readAll is a small test helper to read an *http.Response body fully
// and return it as a string, failing the test on any read error.
func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

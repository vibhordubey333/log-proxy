package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type LogFetcher interface {
	Fetch(ctx context.Context, buildID string) (body io.ReadCloser, contentType string, err error)
}

type JenkinsFetcher struct {
	BaseURL string //"https://ci.jenkins.io"
	Client  *http.Client
}

func (j *JenkinsFetcher) Fetch(ctx context.Context, buildID string) (body io.ReadCloser, contentType string, err error) {
	url := fmt.Sprintf("%s/job/Core/job/jenkins/job/master/%s/consoleText", j.BaseURL, buildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building request: %w", err)
	}
	resp, err := j.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching from jenkins: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, "", ErrBuildNotFound
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		return nil, "", fmt.Errorf("%w: jenkins returned %d", ErrUpstreamUnavailable, resp.StatusCode, snippet)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

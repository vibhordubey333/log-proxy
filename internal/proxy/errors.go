package proxy

import "errors"

var (
	ErrBuildNotFound       = errors.New("build not found upstream")
	ErrUpstreamUnavailable = errors.New("upstream jenkins unavailable")
	ErrInvalidBuildID      = errors.New("invalid build id")
)

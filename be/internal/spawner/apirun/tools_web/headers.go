package tools_web

import (
	"net/http"
	"sync/atomic"
)

// Rotating realistic User-Agent + Accept-Language pools applied by direct.go
// per request. This is a headers-only anti-bot posture: it beats crude
// UA/header allow/deny lists, nothing more — it does not (and cannot)
// address TLS/HTTP2 fingerprinting or JS-challenge walls; see direct.go's
// challenge-marker detection and the RoundTripper seam in egress.go for the
// documented ceiling.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:130.0) Gecko/20100101 Firefox/130.0",
}

var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"en-US,en;q=0.8,fr;q=0.6",
	"en-US,en;q=0.9,de;q=0.7",
}

// headerRotation is a process-wide round-robin counter; using it instead of
// pure rand keeps successive requests within one process from repeating the
// same UA back-to-back, without needing per-provider state.
var headerRotation uint64

// applyRotatingHeaders sets a realistic, rotating User-Agent/Accept/
// Accept-Language on req. Called once per direct.go fetch.
func applyRotatingHeaders(req *http.Request) {
	i := atomic.AddUint64(&headerRotation, 1)
	req.Header.Set("User-Agent", userAgents[int(i)%len(userAgents)])
	req.Header.Set("Accept-Language", acceptLanguages[int(i)%len(acceptLanguages)])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
}

package tools_web

import (
	"fmt"
	"net/http"
	"time"
)

// newHTTPClient builds an http.Client that refuses cross-host redirects. Even
// though providers connect to fixed trusted hosts, a 3xx to an internal address
// would otherwise bypass ValidateFetchURL (which only checks the initial URL) —
// a redirect-SSRF vector. Same-host redirects (e.g. path or scheme bumps) are
// still allowed, capped at a few hops.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			if req.URL.Hostname() != via[0].URL.Hostname() {
				return fmt.Errorf("cross-host redirect to %q blocked", req.URL.Hostname())
			}
			return nil
		},
	}
}

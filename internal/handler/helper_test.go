package handler

import (
	"net/http"
	"net/http/httptest"
)

// redirectTransport forwards requests to the given test server (for handler tests).
type redirectTransport struct{ base *httptest.Server }

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme = "http"
	u.Host = r.base.Listener.Addr().String()
	req2 := req.Clone(req.Context())
	req2.URL = &u
	return http.DefaultTransport.RoundTrip(req2)
}

package instapaper

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

type debugTransport struct {
	base http.RoundTripper
	w    io.Writer
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start)
	if err != nil {
		fmt.Fprintf(t.w, "debug: %s %s error=%v duration=%s\n", req.Method, req.URL.Redacted(), err, dur)
		return nil, err
	}
	fmt.Fprintf(t.w, "debug: %s %s status=%d duration=%s\n", req.Method, req.URL.Redacted(), resp.StatusCode, dur)
	return resp, nil
}

// EnableDebug enables basic HTTP request logging. It never logs headers or bodies.
func (c *Client) EnableDebug(w io.Writer) {
	if c == nil || w == nil {
		return
	}
	base := c.HTTP.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.HTTP.Transport = &debugTransport{base: base, w: w}
}

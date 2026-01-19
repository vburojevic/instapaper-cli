package instapaper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type debugTransport struct {
	base http.RoundTripper
	w    io.Writer
	json bool
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start)
	if t.json {
		payload := map[string]any{
			"type":        "http",
			"method":      req.Method,
			"url":         req.URL.Redacted(),
			"duration_ms": dur.Milliseconds(),
		}
		if err != nil {
			payload["error"] = err.Error()
			_ = writeJSONLine(t.w, payload)
			return nil, err
		}
		payload["status"] = resp.StatusCode
		_ = writeJSONLine(t.w, payload)
		return resp, nil
	}
	if err != nil {
		fmt.Fprintf(t.w, "debug: %s %s error=%v duration=%s\n", req.Method, req.URL.Redacted(), err, dur)
		return nil, err
	}
	fmt.Fprintf(t.w, "debug: %s %s status=%d duration=%s\n", req.Method, req.URL.Redacted(), resp.StatusCode, dur)
	return resp, nil
}

// EnableDebug enables basic HTTP request logging. It never logs headers or bodies.
func (c *Client) EnableDebug(w io.Writer) {
	c.enableDebug(w, false)
}

// EnableDebugJSON enables JSON-line HTTP request logging.
func (c *Client) EnableDebugJSON(w io.Writer) {
	c.enableDebug(w, true)
}

func (c *Client) enableDebug(w io.Writer, json bool) {
	if c == nil || w == nil {
		return
	}
	base := c.HTTP.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.HTTP.Transport = &debugTransport{base: base, w: w, json: json}
}

func writeJSONLine(w io.Writer, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

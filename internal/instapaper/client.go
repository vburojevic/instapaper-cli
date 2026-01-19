package instapaper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vburojevic/instapaper-cli/internal/oauth1"
)

type Client struct {
	BaseURL   string
	Signer    *oauth1.Signer
	Token     *oauth1.Token
	HTTP      *http.Client
	UserAgent string
	RetryCount   int
	RetryBackoff time.Duration
}

func NewClient(baseURL, consumerKey, consumerSecret string, token *oauth1.Token, timeout time.Duration) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("instapaper: baseURL is empty")
	}
	signer := oauth1.NewSigner(consumerKey, consumerSecret)
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	hc := &http.Client{Timeout: timeout}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Signer:    signer,
		Token:     token,
		HTTP:      hc,
		UserAgent: "instapaper-cli/0.1",
	}, nil
}

func (c *Client) SetRetry(count int, backoff time.Duration) {
	if c == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	c.RetryCount = count
	c.RetryBackoff = backoff
}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == 0 {
		return "Instapaper API error"
	}
	if e.Message != "" {
		return fmt.Sprintf("Instapaper API error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("Instapaper API error %d", e.Code)
}

// postForm signs and posts an application/x-www-form-urlencoded request.
// It returns status code, headers, and raw response body.
func (c *Client) postForm(ctx context.Context, path string, form url.Values, accept string) (int, http.Header, []byte, error) {
	attempts := c.RetryCount + 1
	if attempts < 1 {
		attempts = 1
	}
	backoff := c.RetryBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	var lastStatus int
	var lastHeaders http.Header
	var lastBody []byte
	var lastErr error
	for i := 0; i < attempts; i++ {
		status, headers, body, err := c.postFormOnce(ctx, path, form, accept)
		lastStatus, lastHeaders, lastBody, lastErr = status, headers, body, err
		if err == nil && !shouldRetry(status, body) {
			return status, headers, body, nil
		}
		if ctx.Err() != nil {
			return status, headers, body, ctx.Err()
		}
		if i < attempts-1 {
			time.Sleep(backoff * time.Duration(1<<i))
			continue
		}
	}
	return lastStatus, lastHeaders, lastBody, lastErr
}

func (c *Client) postFormOnce(ctx context.Context, path string, form url.Values, accept string) (int, http.Header, []byte, error) {
	fullURL := c.BaseURL + path

	body := form.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	auth, err := c.Signer.AuthorizationHeader(http.MethodPost, fullURL, form, c.Token)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, b, nil
}

func shouldRetry(status int, body []byte) bool {
	if status == 429 || status >= 500 {
		return true
	}
	if apiErr := parseAPIError(body); apiErr != nil {
		return apiErr.Code == 1040
	}
	return false
}

func parseAPIError(body []byte) *APIError {
	// Typical Instapaper errors are returned as a JSON array whose first element has {"type":"error", ...}
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 || trim[0] != '[' {
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(trim, &raw); err != nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	var e struct {
		Type      string `json:"type"`
		ErrorCode int    `json:"error_code"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(raw[0], &e); err != nil {
		return nil
	}
	if e.Type != "error" {
		return nil
	}
	return &APIError{Code: e.ErrorCode, Message: e.Message}
}

func ensureOK(status int, body []byte) error {
	if status >= 200 && status <= 299 {
		if apiErr := parseAPIError(body); apiErr != nil {
			return apiErr
		}
		return nil
	}
	if apiErr := parseAPIError(body); apiErr != nil {
		return apiErr
	}
	return fmt.Errorf("HTTP %d: %s", status, strings.TrimSpace(string(body)))
}

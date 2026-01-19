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

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != 0 {
		return fmt.Sprintf("Instapaper API error %d", e.Code)
	}
	return "Instapaper API error"
}

// postForm signs and posts an application/x-www-form-urlencoded request.
// It returns status code, headers, and raw response body.
func (c *Client) postForm(ctx context.Context, path string, form url.Values, accept string) (int, http.Header, []byte, error) {
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

package instapaper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// XAuthAccessToken exchanges Instapaper username/password for an OAuth access token.
// Note: you must NOT store the user's password; only store the returned token+secret.
func (c *Client) XAuthAccessToken(ctx context.Context, username, password string) (token string, secret string, err error) {
	form := url.Values{}
	form.Set("x_auth_username", username)
	form.Set("x_auth_password", password)
	form.Set("x_auth_mode", "client_auth")

	// This endpoint returns query-string formatted output: oauth_token=...&oauth_token_secret=...
	status, _, b, err := c.postForm(ctx, "/api/1/oauth/access_token", form, "text/plain")
	if err != nil {
		return "", "", err
	}
	if err := ensureOK(status, b); err != nil {
		return "", "", err
	}

	vals, err := url.ParseQuery(strings.TrimSpace(string(b)))
	if err != nil {
		return "", "", fmt.Errorf("parse access token response: %w", err)
	}
	token = vals.Get("oauth_token")
	secret = vals.Get("oauth_token_secret")
	if token == "" || secret == "" {
		return "", "", fmt.Errorf("missing oauth_token/oauth_token_secret in response")
	}
	return token, secret, nil
}

func (c *Client) VerifyCredentials(ctx context.Context) (User, error) {
	status, _, b, err := c.postForm(ctx, "/api/1/account/verify_credentials", url.Values{}, "application/json")
	if err != nil {
		return User{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return User{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return User{}, err
	}
	if len(items) == 0 {
		return User{}, errors.New("empty response")
	}
	var u User
	if err := json.Unmarshal(items[0], &u); err != nil {
		return User{}, err
	}
	return u, nil
}

type AddBookmarkRequest struct {
	URL             string
	Title           string
	Description     string
	FolderID        string
	ResolveFinalURL bool
	Archived        bool
	Tags            []string
	Content         string
	PrivateSource   string // is_private_from_source
}

func (c *Client) AddBookmark(ctx context.Context, req AddBookmarkRequest) (Bookmark, error) {
	form := url.Values{}
	if req.PrivateSource != "" {
		form.Set("is_private_from_source", req.PrivateSource)
		if req.Content == "" {
			return Bookmark{}, &APIError{Code: 1245, Message: "Private bookmarks require supplied content"}
		}
		form.Set("content", req.Content)
	} else {
		form.Set("url", req.URL)
		if req.Content != "" {
			form.Set("content", req.Content)
		}
	}
	if req.Title != "" {
		form.Set("title", req.Title)
	}
	if req.Description != "" {
		form.Set("description", req.Description)
	}
	if req.FolderID != "" {
		form.Set("folder_id", req.FolderID)
	}
	if req.ResolveFinalURL {
		form.Set("resolve_final_url", "1")
	} else {
		form.Set("resolve_final_url", "0")
	}
	if req.Archived {
		form.Set("archived", "1")
	}
	if len(req.Tags) > 0 {
		tags := make([]map[string]string, 0, len(req.Tags))
		for _, t := range req.Tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			tags = append(tags, map[string]string{"name": t})
		}
		if len(tags) > 0 {
			j, err := json.Marshal(tags)
			if err != nil {
				return Bookmark{}, err
			}
			form.Set("tags", string(j))
		}
	}

	status, _, b, err := c.postForm(ctx, "/api/1/bookmarks/add", form, "application/json")
	if err != nil {
		return Bookmark{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Bookmark{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Bookmark{}, err
	}
	if len(items) == 0 {
		return Bookmark{}, errors.New("empty response")
	}
	var bm Bookmark
	if err := json.Unmarshal(items[0], &bm); err != nil {
		return Bookmark{}, err
	}
	return bm, nil
}

type ListBookmarksOptions struct {
	Limit      int
	FolderID   string
	Tag        string
	Have       string
	Highlights string
}

func (c *Client) ListBookmarks(ctx context.Context, opts ListBookmarksOptions) (BookmarksListResponse, error) {
	form := url.Values{}
	if opts.Limit > 0 {
		form.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.FolderID != "" {
		form.Set("folder_id", opts.FolderID)
	}
	if opts.Tag != "" {
		form.Set("tag", opts.Tag)
	}
	if opts.Have != "" {
		form.Set("have", opts.Have)
	}
	if opts.Highlights != "" {
		form.Set("highlights", opts.Highlights)
	}
	status, _, b, err := c.postForm(ctx, "/api/1/bookmarks/list", form, "application/json")
	if err != nil {
		return BookmarksListResponse{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return BookmarksListResponse{}, err
	}
	return parseBookmarksListResponse(b)
}

func parseBookmarksListResponse(b []byte) (BookmarksListResponse, error) {
	trim := bytes.TrimSpace(b)
	if len(trim) == 0 {
		return BookmarksListResponse{}, errors.New("empty body")
	}
	switch trim[0] {
	case '{':
		var resp BookmarksListResponse
		if err := json.Unmarshal(trim, &resp); err != nil {
			return BookmarksListResponse{}, err
		}
		return resp, nil
	case '[':
		items, err := decodeArray(trim)
		if err != nil {
			return BookmarksListResponse{}, err
		}
		var resp BookmarksListResponse
		for _, it := range items {
			var kind struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(it, &kind); err != nil {
				return BookmarksListResponse{}, err
			}
			switch kind.Type {
			case "user":
				var u User
				if err := json.Unmarshal(it, &u); err != nil {
					return BookmarksListResponse{}, err
				}
				resp.User = u
			case "bookmark":
				var bm Bookmark
				if err := json.Unmarshal(it, &bm); err != nil {
					return BookmarksListResponse{}, err
				}
				resp.Bookmarks = append(resp.Bookmarks, bm)
			case "highlight":
				var h Highlight
				if err := json.Unmarshal(it, &h); err != nil {
					return BookmarksListResponse{}, err
				}
				resp.Highlights = append(resp.Highlights, h)
			case "delete":
				var d struct {
					DeleteIDs []Int64 `json:"delete_ids"`
				}
				if err := json.Unmarshal(it, &d); err != nil {
					return BookmarksListResponse{}, err
				}
				resp.DeleteIDs = append(resp.DeleteIDs, d.DeleteIDs...)
			default:
				// Unknown type (e.g. meta); ignore to be forward-compatible.
			}
		}
		return resp, nil
	default:
		return BookmarksListResponse{}, errors.New("invalid JSON response")
	}
}

func (c *Client) UpdateReadProgress(ctx context.Context, bookmarkID int64, progress float64, progressTimestamp int64) (Bookmark, error) {
	form := url.Values{}
	form.Set("bookmark_id", strconv.FormatInt(bookmarkID, 10))
	form.Set("progress", strconv.FormatFloat(progress, 'f', -1, 64))
	form.Set("progress_timestamp", strconv.FormatInt(progressTimestamp, 10))
	status, _, b, err := c.postForm(ctx, "/api/1/bookmarks/update_read_progress", form, "application/json")
	if err != nil {
		return Bookmark{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Bookmark{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Bookmark{}, err
	}
	if len(items) == 0 {
		return Bookmark{}, errors.New("empty response")
	}
	var bm Bookmark
	if err := json.Unmarshal(items[0], &bm); err != nil {
		return Bookmark{}, err
	}
	return bm, nil
}

func (c *Client) DeleteBookmark(ctx context.Context, bookmarkID int64) error {
	form := url.Values{}
	form.Set("bookmark_id", strconv.FormatInt(bookmarkID, 10))
	status, _, b, err := c.postForm(ctx, "/api/1/bookmarks/delete", form, "application/json")
	if err != nil {
		return err
	}
	return ensureOK(status, b)
}

func (c *Client) Star(ctx context.Context, bookmarkID int64) (Bookmark, error) {
	return c.simpleBookmarkMutation(ctx, "/api/1/bookmarks/star", bookmarkID)
}

func (c *Client) Unstar(ctx context.Context, bookmarkID int64) (Bookmark, error) {
	return c.simpleBookmarkMutation(ctx, "/api/1/bookmarks/unstar", bookmarkID)
}

func (c *Client) Archive(ctx context.Context, bookmarkID int64) (Bookmark, error) {
	return c.simpleBookmarkMutation(ctx, "/api/1/bookmarks/archive", bookmarkID)
}

func (c *Client) Unarchive(ctx context.Context, bookmarkID int64) (Bookmark, error) {
	return c.simpleBookmarkMutation(ctx, "/api/1/bookmarks/unarchive", bookmarkID)
}

func (c *Client) Move(ctx context.Context, bookmarkID int64, folderID string) (Bookmark, error) {
	form := url.Values{}
	form.Set("bookmark_id", strconv.FormatInt(bookmarkID, 10))
	form.Set("folder_id", folderID)
	status, _, b, err := c.postForm(ctx, "/api/1/bookmarks/move", form, "application/json")
	if err != nil {
		return Bookmark{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Bookmark{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Bookmark{}, err
	}
	if len(items) == 0 {
		return Bookmark{}, errors.New("empty response")
	}
	var bm Bookmark
	if err := json.Unmarshal(items[0], &bm); err != nil {
		return Bookmark{}, err
	}
	return bm, nil
}

func (c *Client) GetTextHTML(ctx context.Context, bookmarkID int64) ([]byte, error) {
	form := url.Values{}
	form.Set("bookmark_id", strconv.FormatInt(bookmarkID, 10))
	status, headers, b, err := c.postForm(ctx, "/api/1/bookmarks/get_text", form, "text/html")
	if err != nil {
		return nil, err
	}
	// On error, the API returns JSON error structure with HTTP 400.
	if status < 200 || status > 299 {
		if apiErr := parseAPIError(b); apiErr != nil {
			return nil, apiErr
		}
		return nil, fmt.Errorf("HTTP %d: %s", status, strings.TrimSpace(string(b)))
	}
	ct := headers.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		// Sometimes proxies might alter it; still return body.
	}
	return b, nil
}

// Folder methods
func (c *Client) ListFolders(ctx context.Context) ([]Folder, error) {
	status, _, b, err := c.postForm(ctx, "/api/1/folders/list", url.Values{}, "application/json")
	if err != nil {
		return nil, err
	}
	if err := ensureOK(status, b); err != nil {
		return nil, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(items))
	for _, it := range items {
		var f Folder
		if err := json.Unmarshal(it, &f); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, nil
}

func (c *Client) AddFolder(ctx context.Context, title string) (Folder, error) {
	form := url.Values{}
	form.Set("title", title)
	status, _, b, err := c.postForm(ctx, "/api/1/folders/add", form, "application/json")
	if err != nil {
		return Folder{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Folder{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Folder{}, err
	}
	if len(items) == 0 {
		return Folder{}, errors.New("empty response")
	}
	var f Folder
	if err := json.Unmarshal(items[0], &f); err != nil {
		return Folder{}, err
	}
	return f, nil
}

func (c *Client) DeleteFolder(ctx context.Context, folderID int64) error {
	form := url.Values{}
	form.Set("folder_id", strconv.FormatInt(folderID, 10))
	status, _, b, err := c.postForm(ctx, "/api/1/folders/delete", form, "application/json")
	if err != nil {
		return err
	}
	return ensureOK(status, b)
}

func (c *Client) SetFolderOrder(ctx context.Context, order string) ([]Folder, error) {
	form := url.Values{}
	form.Set("order", order)
	status, _, b, err := c.postForm(ctx, "/api/1/folders/set_order", form, "application/json")
	if err != nil {
		return nil, err
	}
	if err := ensureOK(status, b); err != nil {
		return nil, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(items))
	for _, it := range items {
		var f Folder
		if err := json.Unmarshal(it, &f); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// Highlight methods
func (c *Client) ListHighlights(ctx context.Context, bookmarkID int64) ([]Highlight, error) {
	path := fmt.Sprintf("/api/1.1/bookmarks/%d/highlights", bookmarkID)
	status, _, b, err := c.postForm(ctx, path, url.Values{}, "application/json")
	if err != nil {
		return nil, err
	}
	if err := ensureOK(status, b); err != nil {
		return nil, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return nil, err
	}
	hls := make([]Highlight, 0, len(items))
	for _, it := range items {
		var h Highlight
		if err := json.Unmarshal(it, &h); err != nil {
			return nil, err
		}
		hls = append(hls, h)
	}
	return hls, nil
}

func (c *Client) CreateHighlight(ctx context.Context, bookmarkID int64, text string, position int) (Highlight, error) {
	path := fmt.Sprintf("/api/1.1/bookmarks/%d/highlight", bookmarkID)
	form := url.Values{}
	form.Set("text", text)
	if position >= 0 {
		form.Set("position", strconv.Itoa(position))
	}
	status, _, b, err := c.postForm(ctx, path, form, "application/json")
	if err != nil {
		return Highlight{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Highlight{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Highlight{}, err
	}
	if len(items) == 0 {
		return Highlight{}, errors.New("empty response")
	}
	var h Highlight
	if err := json.Unmarshal(items[0], &h); err != nil {
		return Highlight{}, err
	}
	return h, nil
}

func (c *Client) DeleteHighlight(ctx context.Context, highlightID int64) error {
	path := fmt.Sprintf("/api/1.1/highlights/%d/delete", highlightID)
	status, _, b, err := c.postForm(ctx, path, url.Values{}, "application/json")
	if err != nil {
		return err
	}
	return ensureOK(status, b)
}

func (c *Client) simpleBookmarkMutation(ctx context.Context, path string, bookmarkID int64) (Bookmark, error) {
	form := url.Values{}
	form.Set("bookmark_id", strconv.FormatInt(bookmarkID, 10))
	status, _, b, err := c.postForm(ctx, path, form, "application/json")
	if err != nil {
		return Bookmark{}, err
	}
	if err := ensureOK(status, b); err != nil {
		return Bookmark{}, err
	}
	items, err := decodeArray(b)
	if err != nil {
		return Bookmark{}, err
	}
	if len(items) == 0 {
		return Bookmark{}, errors.New("empty response")
	}
	var bm Bookmark
	if err := json.Unmarshal(items[0], &bm); err != nil {
		return Bookmark{}, err
	}
	return bm, nil
}

func decodeArray(b []byte) ([]json.RawMessage, error) {
	trim := strings.TrimSpace(string(b))
	if trim == "" {
		return nil, errors.New("empty body")
	}
	if !strings.HasPrefix(trim, "[") {
		return nil, errors.New("expected JSON array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if apiErr := parseAPIError(b); apiErr != nil {
			return nil, apiErr
		}
	}
	return items, nil
}

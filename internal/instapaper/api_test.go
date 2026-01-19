package instapaper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vburojevic/instapaper-cli/internal/oauth1"
)

func newTestClient(t *testing.T, baseURL string, token *oauth1.Token) *Client {
	t.Helper()
	c, err := NewClient(baseURL, "ck", "cs", token, 2*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func requireAuthHeader(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") == "" {
		t.Fatalf("expected Authorization header")
	}
}

func readForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return r.Form
}

func TestXAuthAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.URL.Path != "/api/1/oauth/access_token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		io.WriteString(w, "oauth_token=tok&oauth_token_secret=sec")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, nil)
	tok, sec, err := client.XAuthAccessToken(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("XAuthAccessToken: %v", err)
	}
	if tok != "tok" || sec != "sec" {
		t.Fatalf("unexpected tokens: %s %s", tok, sec)
	}
}

func TestVerifyCredentials(t *testing.T) {
	resp := []map[string]any{{
		"type":     "user",
		"user_id":  123,
		"username": "vedran",
	}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/account/verify_credentials" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	u, err := client.VerifyCredentials(context.Background())
	if err != nil {
		t.Fatalf("VerifyCredentials: %v", err)
	}
	if int64(u.UserID) != 123 || u.Username != "vedran" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestListBookmarksObjectResponse(t *testing.T) {
	resp := map[string]any{
		"user": map[string]any{"user_id": 1, "username": "u"},
		"bookmarks": []map[string]any{{
			"type":        "bookmark",
			"bookmark_id": 10,
			"url":         "https://example.com",
			"title":       "Example",
			"starred":     1,
			"progress":    0.5,
		}},
		"highlights": []map[string]any{{
			"type":         "highlight",
			"highlight_id": 5,
			"bookmark_id":  10,
			"text":         "hi",
			"time":         0,
			"position":     0,
		}},
		"delete_ids": []int{7, 8},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/list" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		form := readForm(t, r)
		if form.Get("limit") != "3" {
			t.Fatalf("limit=%s", form.Get("limit"))
		}
		if form.Get("folder_id") != "unread" {
			t.Fatalf("folder_id=%s", form.Get("folder_id"))
		}
		if form.Get("have") != "1:0:0" {
			t.Fatalf("have=%s", form.Get("have"))
		}
		if form.Get("highlights") != "10" {
			t.Fatalf("highlights=%s", form.Get("highlights"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	respParsed, err := client.ListBookmarks(context.Background(), ListBookmarksOptions{
		Limit:      3,
		FolderID:   "unread",
		Have:       "1:0:0",
		Highlights: "10",
	})
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if int64(respParsed.User.UserID) != 1 {
		t.Fatalf("user_id=%d", respParsed.User.UserID)
	}
	if len(respParsed.Bookmarks) != 1 || int64(respParsed.Bookmarks[0].BookmarkID) != 10 {
		t.Fatalf("bookmarks=%+v", respParsed.Bookmarks)
	}
	if len(respParsed.Highlights) != 1 || int64(respParsed.Highlights[0].HighlightID) != 5 {
		t.Fatalf("highlights=%+v", respParsed.Highlights)
	}
	if len(respParsed.DeleteIDs) != 2 {
		t.Fatalf("delete_ids=%+v", respParsed.DeleteIDs)
	}
}

func TestListBookmarksArrayResponse(t *testing.T) {
	resp := []map[string]any{
		{"type": "user", "user_id": 1, "username": "u"},
		{"type": "bookmark", "bookmark_id": 9, "url": "https://example.com"},
		{"type": "highlight", "highlight_id": 3, "bookmark_id": 9, "text": "hi", "time": 0, "position": 0},
		{"type": "delete", "delete_ids": []int{4, 5}},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/list" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	respParsed, err := client.ListBookmarks(context.Background(), ListBookmarksOptions{})
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if int64(respParsed.Bookmarks[0].BookmarkID) != 9 {
		t.Fatalf("bookmark_id=%d", respParsed.Bookmarks[0].BookmarkID)
	}
	if len(respParsed.DeleteIDs) != 2 {
		t.Fatalf("delete_ids=%+v", respParsed.DeleteIDs)
	}
}

func TestListBookmarksAPIErrorArray(t *testing.T) {
	errBody := `[ { "type": "error", "error_code": 1240, "message": "Invalid" } ]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, errBody)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.ListBookmarks(context.Background(), ListBookmarksOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
}

func TestAddBookmarkTags(t *testing.T) {
	resp := []map[string]any{{"type": "bookmark", "bookmark_id": 101}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/add" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		form := readForm(t, r)
		if form.Get("url") != "https://example.com" {
			t.Fatalf("url=%s", form.Get("url"))
		}
		var tags []map[string]string
		if err := json.Unmarshal([]byte(form.Get("tags")), &tags); err != nil {
			t.Fatalf("tags json: %v", err)
		}
		if len(tags) != 2 || tags[0]["name"] != "a" || tags[1]["name"] != "b" {
			t.Fatalf("tags=%v", tags)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.AddBookmark(context.Background(), AddBookmarkRequest{
		URL:  "https://example.com",
		Tags: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
}

func TestUpdateReadProgress(t *testing.T) {
	resp := []map[string]any{{"type": "bookmark", "bookmark_id": 1}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/update_read_progress" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		form := readForm(t, r)
		if form.Get("bookmark_id") != "1" {
			t.Fatalf("bookmark_id=%s", form.Get("bookmark_id"))
		}
		if form.Get("progress") != "0.5" {
			t.Fatalf("progress=%s", form.Get("progress"))
		}
		if form.Get("progress_timestamp") != "1700000000" {
			t.Fatalf("progress_timestamp=%s", form.Get("progress_timestamp"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.UpdateReadProgress(context.Background(), 1, 0.5, 1700000000)
	if err != nil {
		t.Fatalf("UpdateReadProgress: %v", err)
	}
}

func TestSimpleBookmarkMutations(t *testing.T) {
	cases := []struct {
		name string
		path string
		fn   func(c *Client) error
	}{
		{
			name: "archive",
			path: "/api/1/bookmarks/archive",
			fn: func(c *Client) error {
				_, err := c.Archive(context.Background(), 1)
				return err
			},
		},
		{
			name: "unarchive",
			path: "/api/1/bookmarks/unarchive",
			fn: func(c *Client) error {
				_, err := c.Unarchive(context.Background(), 1)
				return err
			},
		},
		{
			name: "star",
			path: "/api/1/bookmarks/star",
			fn: func(c *Client) error {
				_, err := c.Star(context.Background(), 1)
				return err
			},
		},
		{
			name: "unstar",
			path: "/api/1/bookmarks/unstar",
			fn: func(c *Client) error {
				_, err := c.Unstar(context.Background(), 1)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := []map[string]any{{"type": "bookmark", "bookmark_id": 1}}
			body, _ := json.Marshal(resp)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("path=%s", r.URL.Path)
				}
				requireAuthHeader(t, r)
				form := readForm(t, r)
				if form.Get("bookmark_id") != "1" {
					t.Fatalf("bookmark_id=%s", form.Get("bookmark_id"))
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(body)
			}))
			defer srv.Close()

			client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
			if err := tc.fn(client); err != nil {
				t.Fatalf("mutation failed: %v", err)
			}
		})
	}
}

func TestMoveBookmark(t *testing.T) {
	resp := []map[string]any{{"type": "bookmark", "bookmark_id": 1}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/move" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		form := readForm(t, r)
		if form.Get("bookmark_id") != "1" || form.Get("folder_id") != "2" {
			t.Fatalf("form=%v", form)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.Move(context.Background(), 1, "2")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
}

func TestDeleteBookmark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/delete" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		form := readForm(t, r)
		if form.Get("bookmark_id") != "1" {
			t.Fatalf("bookmark_id=%s", form.Get("bookmark_id"))
		}
		io.WriteString(w, "[]")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	if err := client.DeleteBookmark(context.Background(), 1); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
}

func TestGetTextHTML(t *testing.T) {
	content := "<html>ok</html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/get_text" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		requireAuthHeader(t, r)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, content)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	b, err := client.GetTextHTML(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetTextHTML: %v", err)
	}
	if string(b) != content {
		t.Fatalf("body=%s", string(b))
	}
}

func TestFoldersEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireAuthHeader(t, r)
		switch r.URL.Path {
		case "/api/1/folders/list":
			resp := []map[string]any{{"type": "folder", "folder_id": 1, "title": "A", "position": 1}}
			body, _ := json.Marshal(resp)
			w.Write(body)
		case "/api/1/folders/add":
			form := readForm(t, r)
			if form.Get("title") != "New" {
				t.Fatalf("title=%s", form.Get("title"))
			}
			resp := []map[string]any{{"type": "folder", "folder_id": 2, "title": "New", "position": 2}}
			body, _ := json.Marshal(resp)
			w.Write(body)
		case "/api/1/folders/delete":
			form := readForm(t, r)
			if form.Get("folder_id") != "2" {
				t.Fatalf("folder_id=%s", form.Get("folder_id"))
			}
			io.WriteString(w, "[]")
		case "/api/1/folders/set_order":
			form := readForm(t, r)
			if form.Get("order") != "1:1,2:2" {
				t.Fatalf("order=%s", form.Get("order"))
			}
			resp := []map[string]any{{"type": "folder", "folder_id": 1, "title": "A", "position": 1}}
			body, _ := json.Marshal(resp)
			w.Write(body)
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	if _, err := client.ListFolders(context.Background()); err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if _, err := client.AddFolder(context.Background(), "New"); err != nil {
		t.Fatalf("AddFolder: %v", err)
	}
	if err := client.DeleteFolder(context.Background(), 2); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if _, err := client.SetFolderOrder(context.Background(), "1:1,2:2"); err != nil {
		t.Fatalf("SetFolderOrder: %v", err)
	}
}

func TestHighlightsEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireAuthHeader(t, r)
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/1.1/bookmarks/") && strings.HasSuffix(r.URL.Path, "/highlights"):
			resp := []map[string]any{{"type": "highlight", "highlight_id": 1, "bookmark_id": 9, "text": "hi", "time": 0, "position": 0}}
			body, _ := json.Marshal(resp)
			w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/api/1.1/bookmarks/") && strings.HasSuffix(r.URL.Path, "/highlight"):
			form := readForm(t, r)
			if form.Get("text") != "quote" {
				t.Fatalf("text=%s", form.Get("text"))
			}
			resp := []map[string]any{{"type": "highlight", "highlight_id": 2, "bookmark_id": 9, "text": "quote", "time": 0, "position": 0}}
			body, _ := json.Marshal(resp)
			w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/api/1.1/highlights/") && strings.HasSuffix(r.URL.Path, "/delete"):
			io.WriteString(w, "[]")
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	if _, err := client.ListHighlights(context.Background(), 9); err != nil {
		t.Fatalf("ListHighlights: %v", err)
	}
	if _, err := client.CreateHighlight(context.Background(), 9, "quote", 0); err != nil {
		t.Fatalf("CreateHighlight: %v", err)
	}
	if err := client.DeleteHighlight(context.Background(), 2); err != nil {
		t.Fatalf("DeleteHighlight: %v", err)
	}
}

func TestDecodeArrayRejectsNonArray(t *testing.T) {
	if _, err := decodeArray([]byte(`{"type":"bookmark"}`)); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseBookmarksListResponseRejectsInvalidJSON(t *testing.T) {
	if _, err := parseBookmarksListResponse([]byte("invalid")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAddBookmarkPrivateSourceRequiresContent(t *testing.T) {
	client := newTestClient(t, "https://example.com", &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.AddBookmark(context.Background(), AddBookmarkRequest{PrivateSource: "src"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestTagsTrimmed(t *testing.T) {
	resp := []map[string]any{{"type": "bookmark", "bookmark_id": 101}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		var tags []map[string]string
		if err := json.Unmarshal([]byte(form.Get("tags")), &tags); err != nil {
			t.Fatalf("tags json: %v", err)
		}
		if len(tags) != 2 {
			t.Fatalf("tags=%v", tags)
		}
		if tags[0]["name"] != "a" || tags[1]["name"] != "b" {
			t.Fatalf("tags=%v", tags)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.AddBookmark(context.Background(), AddBookmarkRequest{
		URL:  "https://example.com",
		Tags: []string{" a ", "", "b"},
	})
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
}

func TestListBookmarksTagIgnoresFolder(t *testing.T) {
	resp := map[string]any{"user": map[string]any{"user_id": 1, "username": "u"}}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		if form.Get("tag") != "news" {
			t.Fatalf("tag=%s", form.Get("tag"))
		}
		if form.Get("folder_id") != "" {
			t.Fatalf("folder_id should be empty, got %s", form.Get("folder_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL, &oauth1.Token{Key: "tok", Secret: "sec"})
	_, err := client.ListBookmarks(context.Background(), ListBookmarksOptions{Tag: "news"})
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
}

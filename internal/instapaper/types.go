package instapaper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Int64 is an int64 that can unmarshal from JSON number or string.
type Int64 int64

func (i *Int64) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*i = 0
		return nil
	}
	// String
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*i = 0
			return nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("parse int64 from %q: %w", s, err)
		}
		*i = Int64(v)
		return nil
	}
	// Number
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	v, err := n.Int64()
	if err != nil {
		return fmt.Errorf("parse int64 from %q: %w", n.String(), err)
	}
	*i = Int64(v)
	return nil
}

// Float64 is a float64 that can unmarshal from JSON number or string.
type Float64 float64

func (f *Float64) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*f = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("parse float64 from %q: %w", s, err)
		}
		*f = Float64(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	v, err := n.Float64()
	if err != nil {
		return fmt.Errorf("parse float64 from %q: %w", n.String(), err)
	}
	*f = Float64(v)
	return nil
}

// BoolInt is a bool that can unmarshal from JSON bool, number (0/1), or string ("0"/"1").
type BoolInt bool

func (bi *BoolInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*bi = false
		return nil
	}
	switch {
	case bytes.Equal(b, []byte("true")):
		*bi = true
		return nil
	case bytes.Equal(b, []byte("false")):
		*bi = false
		return nil
	case b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "1" || s == "true" {
			*bi = true
			return nil
		}
		*bi = false
		return nil
	default:
		var n json.Number
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		iv, err := n.Int64()
		if err != nil {
			return err
		}
		*bi = (iv != 0)
		return nil
	}
}

type User struct {
	Type     string `json:"type"`
	UserID   Int64  `json:"user_id"`
	Username string `json:"username"`
}

type Tag struct {
	ID   Int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

type Bookmark struct {
	Type              string  `json:"type"`
	BookmarkID        Int64   `json:"bookmark_id"`
	URL               string  `json:"url,omitempty"`
	Title             string  `json:"title,omitempty"`
	Description       string  `json:"description,omitempty"`
	Hash              string  `json:"hash,omitempty"`
	Progress          Float64 `json:"progress,omitempty"`
	ProgressTimestamp Int64   `json:"progress_timestamp,omitempty"`
	Starred           BoolInt `json:"starred,omitempty"`
	PrivateSource     string  `json:"private_source,omitempty"`
	Time              Int64   `json:"time,omitempty"`
	Tags              []Tag   `json:"tags,omitempty"`
}

type Folder struct {
	Type     string  `json:"type"`
	FolderID Int64   `json:"folder_id"`
	Title    string  `json:"title"`
	Position Float64 `json:"position,omitempty"`
}

type Highlight struct {
	Type        string `json:"type"`
	HighlightID Int64  `json:"highlight_id"`
	BookmarkID  Int64  `json:"bookmark_id"`
	Text        string `json:"text"`
	Note        string `json:"note,omitempty"`
	Time        Int64  `json:"time"`
	Position    Int64  `json:"position"`
}

type BookmarksListResponse struct {
	User       User        `json:"user"`
	Bookmarks  []Bookmark  `json:"bookmarks"`
	Highlights []Highlight `json:"highlights"`
	DeleteIDs  []Int64     `json:"delete_ids"`
}

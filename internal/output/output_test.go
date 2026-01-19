package output

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vburojevic/instapaper-cli/internal/instapaper"
)

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(b)
}

func TestPrintBookmarksJSONGolden(t *testing.T) {
	bookmarks := []instapaper.Bookmark{{
		Type:       "bookmark",
		BookmarkID: 1,
		URL:        "https://example.com",
		Title:      "Example",
		Progress:   instapaper.Float64(0.5),
		Starred:    instapaper.BoolInt(true),
	}}
	var buf bytes.Buffer
	if err := PrintBookmarks(&buf, "json", bookmarks); err != nil {
		t.Fatalf("PrintBookmarks: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "bookmarks.json"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintFoldersJSONGolden(t *testing.T) {
	folders := []instapaper.Folder{{
		Type:     "folder",
		FolderID: 2,
		Title:    "Work",
		Position: 3,
	}}
	var buf bytes.Buffer
	if err := PrintFolders(&buf, "json", folders); err != nil {
		t.Fatalf("PrintFolders: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "folders.json"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintHighlightsJSONGolden(t *testing.T) {
	highlights := []instapaper.Highlight{{
		Type:        "highlight",
		HighlightID: 4,
		BookmarkID:  1,
		Text:        "Quote",
		Time:        1700000000,
		Position:    10,
	}}
	var buf bytes.Buffer
	if err := PrintHighlights(&buf, "json", highlights); err != nil {
		t.Fatalf("PrintHighlights: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "highlights.json"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintBookmarksNDJSONGolden(t *testing.T) {
	bookmarks := []instapaper.Bookmark{{
		Type:       "bookmark",
		BookmarkID: 1,
		URL:        "https://example.com",
		Title:      "Example",
		Progress:   instapaper.Float64(0.5),
		Starred:    instapaper.BoolInt(true),
	}}
	var buf bytes.Buffer
	if err := PrintBookmarks(&buf, "ndjson", bookmarks); err != nil {
		t.Fatalf("PrintBookmarks ndjson: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "bookmarks.ndjson"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintFoldersNDJSONGolden(t *testing.T) {
	folders := []instapaper.Folder{{
		Type:     "folder",
		FolderID: 2,
		Title:    "Work",
		Position: 3,
	}}
	var buf bytes.Buffer
	if err := PrintFolders(&buf, "ndjson", folders); err != nil {
		t.Fatalf("PrintFolders ndjson: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "folders.ndjson"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintHighlightsNDJSONGolden(t *testing.T) {
	highlights := []instapaper.Highlight{{
		Type:        "highlight",
		HighlightID: 4,
		BookmarkID:  1,
		Text:        "Quote",
		Time:        1700000000,
		Position:    10,
	}}
	var buf bytes.Buffer
	if err := PrintHighlights(&buf, "ndjson", highlights); err != nil {
		t.Fatalf("PrintHighlights ndjson: %v", err)
	}
	if got, want := buf.String(), readGolden(t, "highlights.ndjson"); got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintPlainOutputs(t *testing.T) {
	bookmarks := []instapaper.Bookmark{{
		Type:       "bookmark",
		BookmarkID: 1,
		URL:        "https://example.com",
		Title:      "Example",
		Progress:   instapaper.Float64(0.5),
		Starred:    instapaper.BoolInt(true),
	}}
	var b bytes.Buffer
	if err := PrintBookmarks(&b, "plain", bookmarks); err != nil {
		t.Fatalf("PrintBookmarks plain: %v", err)
	}
	if got, want := b.String(), readGolden(t, "bookmarks.plain"); got != want {
		t.Fatalf("plain bookmarks mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	folders := []instapaper.Folder{{
		Type:     "folder",
		FolderID: 2,
		Title:    "Work",
		Position: 3,
	}}
	var f bytes.Buffer
	if err := PrintFolders(&f, "plain", folders); err != nil {
		t.Fatalf("PrintFolders plain: %v", err)
	}
	if got, want := f.String(), readGolden(t, "folders.plain"); got != want {
		t.Fatalf("plain folders mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	highlights := []instapaper.Highlight{{
		Type:        "highlight",
		HighlightID: 4,
		BookmarkID:  1,
		Text:        "Quote",
		Time:        1700000000,
		Position:    10,
	}}
	var h bytes.Buffer
	if err := PrintHighlights(&h, "plain", highlights); err != nil {
		t.Fatalf("PrintHighlights plain: %v", err)
	}
	if got, want := h.String(), readGolden(t, "highlights.plain"); got != want {
		t.Fatalf("plain highlights mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

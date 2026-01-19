package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/example/instapaper-cli/internal/instapaper"
)

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func PrintBookmarks(w io.Writer, format string, bookmarks []instapaper.Bookmark) error {
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, bookmarks)
	case strings.EqualFold(format, "plain"):
		for _, b := range bookmarks {
			star := "0"
			if bool(b.Starred) {
				star = "1"
			}
			fmt.Fprintf(w, "%d\t%s\t%.4f\t%s\t%s\n",
				int64(b.BookmarkID),
				star,
				float64(b.Progress),
				oneLine(b.Title),
				oneLine(b.URL),
			)
		}
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTAR\tPROG\tTITLE\tURL")
	for _, b := range bookmarks {
		star := ""
		if bool(b.Starred) {
			star = "*"
		}
		prog := float64(b.Progress)
		title := truncateOneLine(b.Title, 60)
		url := truncateOneLine(b.URL, 60)
		fmt.Fprintf(tw, "%d\t%s\t%.2f\t%s\t%s\n", int64(b.BookmarkID), star, prog, title, url)
	}
	return tw.Flush()
}

func PrintFolders(w io.Writer, format string, folders []instapaper.Folder) error {
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, folders)
	case strings.EqualFold(format, "plain"):
		for _, f := range folders {
			fmt.Fprintf(w, "%d\t%d\t%s\n", int64(f.FolderID), int64(f.Position), oneLine(f.Title))
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPOSITION\tTITLE")
	for _, f := range folders {
		fmt.Fprintf(tw, "%d\t%d\t%s\n", int64(f.FolderID), int64(f.Position), truncateOneLine(f.Title, 80))
	}
	return tw.Flush()
}

func PrintHighlights(w io.Writer, format string, highlights []instapaper.Highlight) error {
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, highlights)
	case strings.EqualFold(format, "plain"):
		for _, h := range highlights {
			fmt.Fprintf(w, "%d\t%d\t%d\t%s\n",
				int64(h.HighlightID),
				int64(h.BookmarkID),
				int64(h.Position),
				oneLine(h.Text),
			)
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tBOOKMARK\tPOSITION\tTEXT")
	for _, h := range highlights {
		text := truncateOneLine(h.Text, 80)
		fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", int64(h.HighlightID), int64(h.BookmarkID), int64(h.Position), text)
	}
	return tw.Flush()
}

func truncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "..."
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/vburojevic/instapaper-cli/internal/instapaper"
)

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func WriteJSONLine(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func PrintBookmarks(w io.Writer, format string, bookmarks []instapaper.Bookmark) error {
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, bookmarks)
	case isNDJSON(format):
		for _, b := range bookmarks {
			line, err := json.Marshal(b)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(line)); err != nil {
				return err
			}
		}
		return nil
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

func PrintBookmarksWithFields(w io.Writer, format string, bookmarks []instapaper.Bookmark, fieldsCSV string) error {
	fields, err := parseFields(fieldsCSV)
	if err != nil {
		return err
	}
	records := make([]map[string]any, 0, len(bookmarks))
	for _, b := range bookmarks {
		records = append(records, filterFields(bookmarkToMap(b), fields))
	}
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, records)
	case isNDJSON(format):
		for _, rec := range records {
			if err := WriteJSONLine(w, rec); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("fields are only supported for json/ndjson output")
	}
}

func PrintFolders(w io.Writer, format string, folders []instapaper.Folder) error {
	switch {
	case strings.EqualFold(format, "json"):
		return WriteJSON(w, folders)
	case isNDJSON(format):
		for _, f := range folders {
			line, err := json.Marshal(f)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(line)); err != nil {
				return err
			}
		}
		return nil
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
	case isNDJSON(format):
		for _, h := range highlights {
			line, err := json.Marshal(h)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(line)); err != nil {
				return err
			}
		}
		return nil
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

func isNDJSON(format string) bool {
	return strings.EqualFold(format, "ndjson") || strings.EqualFold(format, "jsonl")
}

func parseFields(fieldsCSV string) ([]string, error) {
	if strings.TrimSpace(fieldsCSV) == "" {
		return nil, nil
	}
	raw := strings.Split(fieldsCSV, ",")
	fields := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, f := range raw {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if !isAllowedField(f) {
			return nil, fmt.Errorf("unknown field: %s", f)
		}
		if !seen[f] {
			fields = append(fields, f)
			seen[f] = true
		}
	}
	return fields, nil
}

func isAllowedField(field string) bool {
	switch field {
	case "type", "bookmark_id", "url", "title", "description", "hash", "progress", "progress_timestamp", "starred", "private_source", "time", "tags":
		return true
	default:
		return false
	}
}

func bookmarkToMap(b instapaper.Bookmark) map[string]any {
	return map[string]any{
		"type":               b.Type,
		"bookmark_id":        int64(b.BookmarkID),
		"url":                b.URL,
		"title":              b.Title,
		"description":        b.Description,
		"hash":               b.Hash,
		"progress":           float64(b.Progress),
		"progress_timestamp": int64(b.ProgressTimestamp),
		"starred":            bool(b.Starred),
		"private_source":     b.PrivateSource,
		"time":               int64(b.Time),
		"tags":               b.Tags,
	}
}

func filterFields(m map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return m
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := m[f]; ok {
			out[f] = v
		}
	}
	return out
}

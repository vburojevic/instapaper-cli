package version

import "strings"

// These can be set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func String() string {
	parts := []string{Version}
	if Commit != "" {
		parts = append(parts, "commit="+Commit)
	}
	if Date != "" {
		parts = append(parts, "date="+Date)
	}
	return strings.Join(parts, " ")
}

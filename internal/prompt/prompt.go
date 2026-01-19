package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func ReadLine(r io.Reader, w io.Writer, prompt string) (string, error) {
	if prompt != "" {
		_, _ = fmt.Fprint(w, prompt)
	}
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil {
		// Allow EOF with partial line
		if err == io.EOF {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ReadPassword reads a password from the controlling TTY when possible, with echo disabled on supported platforms.
// If no TTY is available, it falls back to reading from r (which may echo).
func ReadPassword(w io.Writer, prompt string, r io.Reader) (string, error) {
	if prompt != "" {
		_, _ = fmt.Fprint(w, prompt)
	}
	pw, err := readPasswordFromTTY()
	if err == nil {
		_, _ = fmt.Fprintln(w)
		return strings.TrimSpace(string(pw)), nil
	}
	// Fallback to plain read (may echo)
	line, err2 := ReadLine(r, w, "")
	if err2 != nil {
		return "", err2
	}
	return strings.TrimSpace(line), nil
}

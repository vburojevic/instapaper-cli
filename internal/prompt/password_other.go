//go:build !linux && !darwin

package prompt

import "errors"

func readPasswordFromTTY() ([]byte, error) {
	return nil, errors.New("no tty password reader on this platform; use --password-stdin")
}

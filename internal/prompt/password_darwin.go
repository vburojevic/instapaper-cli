//go:build darwin

package prompt

import (
	"bufio"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

func readPasswordFromTTY() ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer tty.Close()

	fd := int(tty.Fd())
	old, err := ioctlGetTermios(fd)
	if err != nil {
		return nil, err
	}
	newState := *old
	newState.Lflag &^= syscall.ECHO
	if err := ioctlSetTermios(fd, &newState); err != nil {
		return nil, err
	}
	defer func() { _ = ioctlSetTermios(fd, old) }()

	r := bufio.NewReader(tty)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	return []byte(line), nil
}

func ioctlGetTermios(fd int) (*syscall.Termios, error) {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	return &t, nil
}

func ioctlSetTermios(fd int, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

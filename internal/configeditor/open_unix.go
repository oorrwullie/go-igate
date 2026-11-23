//go:build linux || darwin

package configeditor

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openRegularNoFollow refuses symbolic links even if the entry changes after
// the caller began opening it. O_NONBLOCK prevents opening a FIFO from hanging.
func openRegularNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	return os.NewFile(uintptr(fd), path), nil
}

//go:build !linux && !darwin

package configeditor

import (
	"errors"
	"os"
)

// Refuse file editing where the no-follow open guarantee is not implemented.
func openRegularNoFollow(string) (*os.File, error) {
	return nil, errors.New("configuration editor storage is supported only on Linux and macOS")
}

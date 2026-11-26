// Command sparky-init-config publishes a private configuration without replacing
// any existing destination entry.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func initialize(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sparky-config-init.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Link(tmp.Name(), destination)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: sparky-init-config SOURCE DESTINATION")
		os.Exit(2)
	}
	if err := initialize(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "Configuration not created:", err)
		os.Exit(1)
	}
}

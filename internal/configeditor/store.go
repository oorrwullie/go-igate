// Package configeditor manages the on-disk YAML document without rewriting
// comments, formatting, or fields unknown to the editor.
package configeditor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/oorrwullie/sparky/internal/config"
)

const MaxDocumentSize = 256 << 10

var ErrConflict = errors.New("configuration changed on disk; reload before saving")

// Store provides optimistic concurrency for one running Sparky process. It
// does not attempt to coordinate concurrent independent editor processes.
type Store struct {
	Path string
	mu sync.Mutex
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Load returns the exact YAML bytes and a revision token. The original
// bytes must not be sent to an unauthenticated or untrusted browser: they may
// contain APRS-IS passcodes, MQTT passwords, and API keys.
func (s *Store) Load() ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() ([]byte, string, error) {
	if s.Path == "" {
		return nil, "", errors.New("configuration path is empty")
	}
	info, err := os.Lstat(s.Path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("configuration must be a regular file (no symlinks)")
	}
	if info.Size() > MaxDocumentSize {
		return nil, "", fmt.Errorf("configuration exceeds %d bytes", MaxDocumentSize)
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxDocumentSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxDocumentSize {
		return nil, "", fmt.Errorf("configuration exceeds %d bytes", MaxDocumentSize)
	}
	return data, digest(data), nil
}

// Save validates the candidate with Sparky's startup parser, checks the
// expected revision and replaces the existing file atomically. The caller
// must require authentication, CSRF protection, and an explicit Save action.
// Saving never applies the new configuration to the running application.
func (s *Store) Save(candidate []byte, expectedRevision string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(candidate) == 0 || len(candidate) > MaxDocumentSize {
		return "", fmt.Errorf("configuration must be 1..%d bytes", MaxDocumentSize)
	}
	if expectedRevision == "" {
		return "", ErrConflict
	}
	if _, err := config.ParseConfigBytes(candidate); err != nil {
		return "", fmt.Errorf("invalid configuration: %w", err)
	}
	_, revision, err := s.load()
	if err != nil {
		return "", err
	}
	if revision != expectedRevision {
		return "", ErrConflict
	}

	dir := filepath.Dir(s.Path)
	f, err := os.CreateTemp(dir, ".sparky-config-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := f.Write(candidate); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	// Check again before replacement to reduce the window for outside edits.
	// Writes from other processes must be serialized by the operator.
	_, revision, err = s.load()
	if err != nil {
		return "", err
	}
	if revision != expectedRevision {
		return "", ErrConflict
	}
	if err := os.Rename(f.Name(), s.Path); err != nil {
		return "", err
	}
	// On Linux this makes the directory entry durable after a crash.
	d, err := os.Open(dir)
	if err != nil {
		return "", err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return "", err
	}
	return digest(candidate), nil
}

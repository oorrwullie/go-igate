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

// ErrDurability means the replacement succeeded, but crash durability could not
// be confirmed. Save returns the new revision alongside this error.
var ErrDurability = errors.New("configuration saved, but directory sync failed; durability unconfirmed")

// Store provides optimistic concurrency for one running Sparky process. It
// does not attempt to coordinate concurrent independent editor processes.
type Store struct {
	Path string
	mu sync.Mutex
	// syncDirectory is injectable for deterministic failure testing.
	syncDirectory func(string) error
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
	// Open atomically with O_NOFOLLOW, O_NONBLOCK and O_CLOEXEC. A separate
	// Lstat check would race with a directory-entry replacement.
	f, err := openRegularNoFollow(s.Path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("configuration must be a regular file (no symlinks)")
	}
	if info.Size() > MaxDocumentSize {
		return nil, "", fmt.Errorf("configuration exceeds %d bytes", MaxDocumentSize)
	}
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
	// The rename has committed the new file. Never report an ordinary failed
	// save after this point: callers need the new revision to avoid retrying
	// against a stale token.
	newRevision := digest(candidate)
	syncDirectory := s.syncDirectory
	if syncDirectory == nil {
		syncDirectory = syncDir
	}
	if err := syncDirectory(dir); err != nil {
		return newRevision, fmt.Errorf("%w: %v", ErrDurability, err)
	}
	return newRevision, nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

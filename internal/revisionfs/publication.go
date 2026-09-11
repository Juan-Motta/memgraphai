// Package revisionfs prepares immutable Markdown revision files.
package revisionfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// ErrAlreadyPublished prevents replacement of an immutable revision file.
	ErrAlreadyPublished = errors.New("revision already published")
	// ErrUnsafeID rejects an identifier that could escape its path component.
	ErrUnsafeID = errors.New("unsafe identifier path component")
)

// Point identifies a filesystem boundary that tests may interrupt.
type Point string

const (
	SyncStagingFile Point = "sync_staging_file"
	BeforePublish   Point = "before_publish"
	SyncRevisionDir Point = "sync_revision_dir"
)

// Fault optionally interrupts a filesystem boundary.
type Fault func(Point) error

// Prepared describes an immutable revision path prepared for SQLite metadata.
type Prepared struct {
	Path     string
	Checksum string
	Bytes    int64
}

// Filesystem confines revision files to one library root.
type Filesystem struct {
	root  string
	fault Fault
}

// New constructs a revision filesystem rooted at root.
func New(root string, fault Fault) *Filesystem {
	return &Filesystem{root: root, fault: fault}
}

// Prepare syncs staged Markdown before publishing it to an immutable ID-based path.
func (f *Filesystem) Prepare(operationID, projectID, documentID, revisionID string, markdown []byte) (Prepared, error) {
	for _, id := range []string{operationID, projectID, documentID, revisionID} {
		if !safeComponent(id) {
			return Prepared{}, fmt.Errorf("%w: %q", ErrUnsafeID, id)
		}
	}

	revisionDir := filepath.Join(f.root, "projects", projectID, "documents", documentID, "revisions")
	stagingDir := filepath.Join(f.root, ".staging", operationID)
	if err := ensureDir(revisionDir); err != nil {
		return Prepared{}, fmt.Errorf("prepare revision directory: %w", err)
	}
	if err := ensureDir(stagingDir); err != nil {
		return Prepared{}, fmt.Errorf("prepare staging directory: %w", err)
	}

	stagingPath := filepath.Join(stagingDir, revisionID+".tmp")
	staging, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Prepared{}, fmt.Errorf("create staged revision: %w", err)
	}
	if _, err := staging.Write(markdown); err != nil {
		staging.Close()
		return Prepared{}, fmt.Errorf("write staged revision: %w", err)
	}
	if err := staging.Sync(); err != nil {
		staging.Close()
		return Prepared{}, fmt.Errorf("sync staged revision: %w", err)
	}
	if err := staging.Close(); err != nil {
		return Prepared{}, fmt.Errorf("close staged revision: %w", err)
	}
	if err := f.interrupt(SyncStagingFile); err != nil {
		return Prepared{}, err
	}
	if err := syncDir(stagingDir); err != nil {
		return Prepared{}, fmt.Errorf("sync staging directory: %w", err)
	}
	if err := f.interrupt(BeforePublish); err != nil {
		return Prepared{}, err
	}

	finalPath := filepath.Join(revisionDir, revisionID+".md")
	if err := os.Link(stagingPath, finalPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Prepared{}, ErrAlreadyPublished
		}
		return Prepared{}, fmt.Errorf("publish immutable revision: %w", err)
	}
	if err := syncDir(revisionDir); err != nil {
		return Prepared{}, fmt.Errorf("sync revision directory: %w", err)
	}
	if err := f.interrupt(SyncRevisionDir); err != nil {
		return Prepared{}, err
	}
	if err := os.Remove(stagingPath); err != nil {
		return Prepared{}, fmt.Errorf("remove staged revision: %w", err)
	}
	if err := syncDir(stagingDir); err != nil {
		return Prepared{}, fmt.Errorf("sync staging cleanup: %w", err)
	}

	digest := sha256.Sum256(markdown)
	return Prepared{Path: finalPath, Checksum: hex.EncodeToString(digest[:]), Bytes: int64(len(markdown))}, nil
}

func (f *Filesystem) interrupt(point Point) error {
	if f.fault == nil {
		return nil
	}
	return f.fault(point)
}

func ensureDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return fmt.Errorf("directory root does not exist: %s", path)
	}
	if err := ensureDir(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := syncDir(parent); err != nil {
		return fmt.Errorf("sync parent directory: %w", err)
	}
	return syncDir(path)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func safeComponent(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

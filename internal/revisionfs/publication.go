// Package revisionfs prepares immutable Markdown revision files.
package revisionfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrAlreadyPublished prevents replacement of an immutable revision file.
	ErrAlreadyPublished = errors.New("revision already published")
	// ErrUnsafeID rejects an identifier that could escape its path component.
	ErrUnsafeID = errors.New("unsafe identifier path component")
	// ErrIntegrity reports an altered or missing immutable revision file.
	ErrIntegrity = errors.New("revision integrity discrepancy")
	// ErrSymlinkBoundary rejects a library directory that escapes through a link.
	ErrSymlinkBoundary = errors.New("symlinked library boundary")
)

// Point identifies a filesystem boundary that tests may interrupt.
type Point string

const (
	SyncStagingFile Point = "sync_staging_file"
	BeforePublish   Point = "before_publish"
	AfterPublish    Point = "after_publish"
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
	return f.prepare(operationID, "", projectID, documentID, revisionID, markdown)
}

// PrepareAttempt isolates a fenced operation attempt in its own staging directory.
// Repeating the same attempt verifies an existing immutable destination rather than
// replacing it, including after an interruption between linking and directory sync.
func (f *Filesystem) PrepareAttempt(operationID string, generation int64, projectID, documentID, revisionID string, markdown []byte) (Prepared, error) {
	return f.prepare(operationID, fmt.Sprintf("%d", generation), projectID, documentID, revisionID, markdown)
}

func (f *Filesystem) prepare(operationID, generation, projectID, documentID, revisionID string, markdown []byte) (Prepared, error) {
	for _, id := range []string{operationID, projectID, documentID, revisionID} {
		if !safeComponent(id) {
			return Prepared{}, fmt.Errorf("%w: %q", ErrUnsafeID, id)
		}
	}
	if err := f.ensureRoot(); err != nil {
		return Prepared{}, err
	}

	revisionDir := filepath.Join(f.root, "projects", projectID, "documents", documentID, "revisions")
	stagingDir := filepath.Join(f.root, ".staging", operationID)
	if generation != "" {
		stagingDir = filepath.Join(stagingDir, generation)
	}
	if err := ensureDir(f.root, revisionDir); err != nil {
		return Prepared{}, fmt.Errorf("prepare revision directory: %w", err)
	}
	if err := ensureDir(f.root, stagingDir); err != nil {
		return Prepared{}, fmt.Errorf("prepare staging directory: %w", err)
	}

	digest := sha256.Sum256(markdown)
	checksum := hex.EncodeToString(digest[:])
	stagingPath := filepath.Join(stagingDir, revisionID+".tmp")
	if err := writeOrVerify(stagingPath, markdown, checksum); err != nil {
		return Prepared{}, fmt.Errorf("prepare staged revision: %w", err)
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
		if !errors.Is(err, os.ErrExist) {
			return Prepared{}, fmt.Errorf("publish immutable revision: %w", err)
		}
		if generation == "" {
			return Prepared{}, ErrAlreadyPublished
		}
	}
	if err := verifyFile(finalPath, checksum, int64(len(markdown))); err != nil {
		return Prepared{}, err
	}
	if err := f.interrupt(AfterPublish); err != nil {
		return Prepared{}, err
	}
	if err := syncDir(revisionDir); err != nil {
		return Prepared{}, fmt.Errorf("sync revision directory: %w", err)
	}
	if err := f.interrupt(SyncRevisionDir); err != nil {
		return Prepared{}, err
	}
	if err := os.Remove(stagingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Prepared{}, fmt.Errorf("remove staged revision: %w", err)
	}
	if err := syncDir(stagingDir); err != nil {
		return Prepared{}, fmt.Errorf("sync staging cleanup: %w", err)
	}
	return Prepared{Path: finalPath, Checksum: checksum, Bytes: int64(len(markdown))}, nil
}

// Verify checks the exact revision selected by SQLite; it never scans or imports.
func Verify(path, checksum string, bytes int64) error {
	_, err := ReadVerified(path, checksum, bytes)
	return err
}

// Orphans returns immutable revision paths not represented by known SQLite metadata.
// It reports only; callers decide no deletion or import policy here.
func (f *Filesystem) Orphans(known map[string]bool) ([]string, error) {
	root := filepath.Join(f.root, "projects")
	orphans := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrSymlinkBoundary
		}
		if !entry.IsDir() && filepath.Ext(path) == ".md" && !known[path] {
			orphans = append(orphans, path)
		}
		return nil
	})
	return orphans, err
}

func (f *Filesystem) ensureRoot() error {
	info, err := os.Lstat(f.root)
	if err == nil {
		return validateDirectory(f.root, info)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	current := f.root
	missing := make([]string, 0)
	for {
		info, err = os.Lstat(current)
		if err == nil {
			if err := validateDirectory(current, info); err != nil {
				return err
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("directory root does not exist: %s", current)
		}
		current = parent
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := createCheckedDirectory(missing[index], os.Mkdir); err != nil {
			return err
		}
	}
	return nil
}

func writeOrVerify(path string, markdown []byte, checksum string) error {
	staging, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return verifyFile(path, checksum, int64(len(markdown)))
	}
	if err != nil {
		return err
	}
	if _, err := staging.Write(markdown); err != nil {
		staging.Close()
		return err
	}
	if err := staging.Sync(); err != nil {
		staging.Close()
		return err
	}
	return staging.Close()
}

func verifyFile(path, checksum string, bytes int64) error {
	return Verify(path, checksum, bytes)
}

func (f *Filesystem) interrupt(point Point) error {
	if f.fault == nil {
		return nil
	}
	return f.fault(point)
}

func ensureDir(root, path string) error {
	return ensureDirWithMkdir(root, path, os.Mkdir)
}

func ensureDirWithMkdir(root, path string, mkdir func(string, os.FileMode) error) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return ErrSymlinkBoundary
	}
	current := root
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := createCheckedDirectory(current, mkdir); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := validateDirectory(current, info); err != nil {
			return err
		}
	}
	return nil
}

func createCheckedDirectory(path string, mkdir func(string, os.FileMode) error) error {
	if err := mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if err := validateDirectory(path, info); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync parent directory: %w", err)
	}
	return syncDir(path)
}

func validateDirectory(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlinkBoundary
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
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

package revisionfs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"
)

// RevisionProvenance is immutable metadata committed with a revision pointer.
type RevisionProvenance struct {
	Origin string
	Client string
	Model  string
}

// DocumentRevision describes an exact SQLite-selected revision before its Markdown is read.
type DocumentRevision struct {
	DocumentID   string
	RevisionID   string
	ProjectID    string
	WorkstreamID string
	Checksum     string
	ByteCount    int64
	Provenance   RevisionProvenance
	CreatedAt    time.Time
	Cursor       int64
}

// ReadVerified returns exact bytes from the regular descriptor whose identity and
// checksum were verified; it never verifies a pathname then reopens it for reading.
func ReadVerified(path, checksum string, bytes int64) ([]byte, error) {
	if bytes < 0 {
		return nil, ErrIntegrity
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != bytes {
		return nil, ErrIntegrity
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrIntegrity
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, ErrIntegrity
	}
	content, err := io.ReadAll(io.LimitReader(file, bytes+1))
	if err != nil || int64(len(content)) != bytes {
		return nil, ErrIntegrity
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return nil, ErrIntegrity
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != checksum {
		return nil, ErrIntegrity
	}
	return content, nil
}

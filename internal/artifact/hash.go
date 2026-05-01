// Package artifact handles tarball fetch, unpack, and content hashing.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// SHA256File returns the lowercase hex sha256 of a file's contents.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SHA256Bytes returns the lowercase hex sha256 of an in-memory byte slice.
func SHA256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TreeHash walks a directory and returns a deterministic hash of its
// contents. Files are sorted by relative path; each file contributes
// "<relpath>\x00<sha256>\n" to the digest.
//
// This is used for .alignment markers — drift detection over a payload
// directory tree.
func TreeHash(root string) (string, error) {
	type entry struct {
		rel string
		sum string
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum, err := SHA256File(path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{rel: rel, sum: sum})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		_, _ = h.Write([]byte(e.rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(e.sum))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

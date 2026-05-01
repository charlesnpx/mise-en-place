package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Unpack extracts a gzipped tarball into dest. It refuses absolute paths and
// paths that escape via "..".
func Unpack(tarball []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if err := validateTarPath(hdr.Name); err != nil {
			return err
		}
		out := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// Symlinks, devices, etc. are not allowed in skill payloads.
			return fmt.Errorf("unsupported tar entry type %v at %s", hdr.Typeflag, hdr.Name)
		}
	}
}

func validateTarPath(name string) error {
	if name == "" {
		return fmt.Errorf("empty tar path")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute tar path: %s", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("path escapes archive: %s", name)
		}
	}
	return nil
}

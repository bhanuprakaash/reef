package store

import "path/filepath"

// ResolvePath returns the absolute path with symbolic links resolved.
func ResolvePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

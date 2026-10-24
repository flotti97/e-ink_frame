// Package atomicfile writes files so that readers never see a partial write,
// even if the Frame loses power midway.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to path via a temporary file and rename, so readers never see a
// partly written file.
func Write(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync() // the Frame may lose power at any time
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

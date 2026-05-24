package pdf

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

func WriteFileAtomic(path string, fn func(tmpPath string) error) error {
	var b [8]byte
	rand.Read(b[:])
	suffix := hex.EncodeToString(b[:])
	tmpPath := path + "." + suffix + ".tmp"

	if err := fn(tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

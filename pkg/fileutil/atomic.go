package fileutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

func WriteFileAtomic(path string, fn func(tmpPath string) error) error {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Errorf("сгенерировать суффикс временного файла: %w", err)
	}
	suffix := hex.EncodeToString(b[:])
	tmpPath := path + "." + suffix + ".tmp"

	if err := fn(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

package fileutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
		// Ошибку при неудачной записи возвращаем как есть, но к ней присоединяем
		// сбой очистки временного файла, чтобы мусор не остался незамеченным.
		return errors.Join(err, os.Remove(tmpPath))
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return errors.Join(err, os.Remove(tmpPath))
	}

	return nil
}

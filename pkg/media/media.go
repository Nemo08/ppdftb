// Package media загружает картинки для подстановки в DOCX-шаблоны:
// из папки с картинками (флаг -p) и из полей-путей в данных документа.
package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"log/slog"
)

// MediaLoader содержит картинки для подстановки в DOCX-шаблоны.
// Static — картинки из папки -p (ключ — имя файла, значение — []byte).
// Mapped — картинки, пути к которым указаны в XML-данных (ключ — имя, значение — []byte).
type MediaLoader struct {
	mu     sync.RWMutex
	Static map[string][]byte
	Mapped map[string][]byte
}

// RangeStatic безопасно перечисляет Static-картинки.
func (m *MediaLoader) RangeStatic(fn func(k string, v []byte) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.Static {
		if !fn(k, v) {
			break
		}
	}
}

// RangeMapped безопасно перечисляет Mapped-картинки.
func (m *MediaLoader) RangeMapped(fn func(k string, v []byte) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.Mapped {
		if !fn(k, v) {
			break
		}
	}
}

// LoadMedia загружает картинки из папки -p и из полей-путей в data.
func LoadMedia(picsDir string, data []byte) *MediaLoader {
	m := &MediaLoader{
		Static: make(map[string][]byte),
		Mapped: make(map[string][]byte),
	}
	loadStaticImages(picsDir, m)
	loadMappedImages(data, m, picsDir)
	return m
}

func loadStaticImages(picsDir string, m *MediaLoader) {
	if picsDir == "" {
		return
	}
	entries, err := os.ReadDir(picsDir)
	if err != nil {
		slog.Default().Debug("не удалось прочитать папку с картинками", slog.String("dir", picsDir), slog.String("err", err.Error()))
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		fullPath := filepath.Join(picsDir, entry.Name())
		//nolint:gosec // чтение изображений из указанного пользователем каталога — ожидаемое поведение
		imageContent, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Default().Warn("не удалось прочитать картинку", slog.String("file", fullPath), slog.String("err", err.Error()))
			continue
		}
		m.mu.Lock()
		m.Static[entry.Name()] = imageContent
		m.mu.Unlock()
		slog.Default().Debug("загружена картинка из -p", slog.String("file", entry.Name()))
	}
}

func loadMappedImages(data []byte, m *MediaLoader, picsDir string) {
	if len(data) == 0 {
		return
	}
	var dataMap map[string]any
	if err := json.Unmarshal(data, &dataMap); err != nil {
		slog.Default().Error("разбор JSON для mapped-картинок", slog.String("err", err.Error()))
		return
	}
	findImageValues(dataMap, m, picsDir)
}

func findImageValues(data any, m *MediaLoader, picsDir string) {
	switch v := data.(type) {
	case string:
		loadImageFromPath("", v, m, picsDir)
	case map[string]any:
		for _, val := range v {
			findImageValues(val, m, picsDir)
		}
	case []any:
		for _, val := range v {
			findImageValues(val, m, picsDir)
		}
	}
}

func loadImageFromPath(key, val string, m *MediaLoader, picsDir string) {
	normalizedVal := strings.ReplaceAll(val, `\`, `/`)
	ext := strings.ToLower(path.Ext(normalizedVal))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return
	}
	base := path.Base(normalizedVal)
	// Уже загружено из picsDir как Static — не орём и не дублируем ReadFile.
	if _, ok := m.Static[base]; ok {
		m.mu.Lock()
		m.Mapped[base] = m.Static[base]
		m.Mapped[val] = m.Static[base]
		m.Mapped[normalizedVal] = m.Static[base]
		m.mu.Unlock()
		return
	}
	candidates := []string{val, filepath.FromSlash(normalizedVal), normalizedVal}
	if picsDir != "" {
		candidates = append(candidates,
			filepath.Join(picsDir, base),
			filepath.Join(picsDir, val),
			filepath.Join(picsDir, filepath.FromSlash(normalizedVal)),
		)
	}
	var imageContent []byte
	var err error
	var used string
	for _, c := range candidates {
		if c == "" {
			continue
		}
		//nolint:gosec // чтение изображений по путям из данных документа — ожидаемое поведение
		imageContent, err = os.ReadFile(c)
		if err == nil {
			used = c
			break
		}
	}
	if err != nil || len(imageContent) == 0 {
		slog.Default().Warn("не удалось прочитать картинку",
			slog.String("key", key), slog.String("file", val), slog.String("picsDir", picsDir), slog.String("err", fmt.Sprintf("%v", err)))
		return
	}
	m.mu.Lock()
	m.Mapped[base] = imageContent
	m.Mapped[val] = imageContent
	m.Mapped[normalizedVal] = imageContent
	if used != val {
		m.Mapped[used] = imageContent
	}
	m.mu.Unlock()
	slog.Default().Debug("загружена картинка из данных", slog.String("key", key), slog.String("file", base), slog.String("from", used))
}

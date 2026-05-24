package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"log/slog"

	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
)

const cacheFileName = ".filecache.json"

// FileEntry — запись об одном файле в кэше.
type FileEntry struct {
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
	Hash    string    `json:"hash,omitempty"`
	Pages   int       `json:"pages,omitempty"` // количество страниц PDF (для toc)
}

// Cache — карта: относительный путь файла → запись.
// Хранится в .filecache.json в рабочей директории.
// Не потокобезопасна — предполагается последовательный доступ.
type Cache map[string]FileEntry

func cachePath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, cacheFileName), nil
}

func toRel(absPath string) string {
	wd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(wd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

func toAbs(relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}
	wd, err := os.Getwd()
	if err != nil {
		return relPath
	}
	return filepath.Join(wd, relPath)
}

func (c Cache) Get(absPath string) (FileEntry, bool) {
	entry, ok := c[toRel(absPath)]
	return entry, ok
}

func (c Cache) Set(absPath string, entry FileEntry) {
	c[toRel(absPath)] = entry
}

// ToRel возвращает относительный путь от рабочей директории.
func ToRel(absPath string) string {
	return toRel(absPath)
}

// ToAbs возвращает абсолютный путь из относительного.
func ToAbs(relPath string) string {
	return toAbs(relPath)
}

// LoadCache загружает кэш из файла.
// Если файл не существует — возвращает пустой кэш без ошибки.
func LoadCache() (Cache, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		slog.Debug("кэш не найден, создаём новый", slog.String("path", path))
		return make(Cache), nil
	}
	if err != nil {
		return nil, err
	}

	var c Cache
	if err = json.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	slog.Debug("кэш загружен", slog.String("path", path), slog.Int("entries", len(c)))
	return c, nil
}

// SaveCache сохраняет кэш в файл (атомарно: tmp + rename).
func SaveCache(c Cache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	if err := pdf.WriteFileAtomic(path, func(tmpPath string) error {
		return os.WriteFile(tmpPath, data, 0o644)
	}); err != nil {
		return err
	}

	slog.Debug("кэш сохранён", slog.String("path", path), slog.Int("entries", len(c)))
	return nil
}

var saveMu sync.Mutex

// SaveCacheAsync сохраняет кэш асинхронно, не блокируя вызывающий код.
// Гарантирует последовательность: при множественных вызовах последнее сохранение
// перезаписывает предыдущие. Делает снапшот карты перед отправкой в горутину.
func SaveCacheAsync(c Cache) {
	snapshot := make(Cache, len(c))
	for k, v := range c {
		snapshot[k] = v
	}
	go func() {
		saveMu.Lock()
		defer saveMu.Unlock()
		if err := SaveCache(snapshot); err != nil {
			slog.Error("асинхронное сохранение кэша", slog.String("err", err.Error()))
		}
	}()
}

// UpdateCache обходит папки из dirs, собирает файлы с расширениями exts
// и добавляет в кэш только те файлы, которых в нём ещё нет.
func UpdateCache(dirs []string, exts map[string]bool, withHash bool) (Cache, error) {
	c, err := LoadCache()
	if err != nil {
		return nil, err
	}

	sizeBefore := len(c)
	pruneCache(c)
	pruned := len(c) != sizeBefore
	if pruned {
		slog.Debug("удалены записи об отсутствующих файлах", slog.Int("count", sizeBefore-len(c)))
	}

	files, err := collectFiles(dirs, exts)
	if err != nil {
		return nil, err
	}

	added := 0
	for _, absPath := range files {
		if _, exists := c.Get(absPath); exists {
			continue
		}
		entry, err := makeEntry(absPath, withHash)
		if err != nil {
			return nil, err
		}
		c.Set(absPath, entry)
		added++
		slog.Debug("добавлен в кэш", slog.String("file", toRel(absPath)))
	}

	if added > 0 || pruned {
		SaveCacheAsync(c)
		slog.Debug("кэш обновлён", slog.Int("added", added), slog.Int("pruned", sizeBefore-len(c)+added))
	} else {
		slog.Debug("кэш актуален, изменений нет")
	}

	return c, nil
}

// CommitCache обходит папки из dirs, собирает файлы с расширениями exts
// и перезаписывает их слепки в кэше актуальными данными.
// Вызывать после успешной обработки файлов.
func CommitCache(dirs []string, exts map[string]bool, withHash bool) (Cache, error) {
	c, err := LoadCache()
	if err != nil {
		return nil, err
	}

	sizeBefore := len(c)
	pruneCache(c)
	if pruned := sizeBefore - len(c); pruned > 0 {
		slog.Debug("удалены записи об отсутствующих файлах", slog.Int("count", pruned))
	}

	files, err := collectFiles(dirs, exts)
	if err != nil {
		return nil, err
	}

	for _, absPath := range files {
		entry, err := makeEntry(absPath, withHash)
		if err != nil {
			return nil, fmt.Errorf("слепок файла %q: %w", absPath, err)
		}
		c.Set(absPath, entry)
		slog.Debug("зафиксирован в кэше", slog.String("file", toRel(absPath)))
	}

	SaveCacheAsync(c)
	slog.Debug("кэш зафиксирован", slog.Int("files", len(files)))
	return c, nil
}

// FilterChanged принимает список файлов и возвращает только те,
// которые изменились по сравнению с кэшем (или отсутствуют в нём).
func FilterChanged(files []string, c Cache, withHash bool) ([]string, error) {
	var changed []string

	for _, path := range files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		cached, exists := c.Get(abs)
		if !exists {
			slog.Debug("новый файл", slog.String("file", toRel(abs)))
			changed = append(changed, path)
			continue
		}

		current, err := makeEntry(abs, withHash)
		if err != nil {
			return nil, err
		}

		if isChanged(cached, current, withHash) {
			if withHash {
				slog.Debug("файл изменился (хэш)",
					slog.String("file", toRel(abs)),
					slog.String("old_hash", cached.Hash[:8]+"..."),
					slog.String("new_hash", current.Hash[:8]+"..."),
				)
			} else {
				slog.Debug("файл изменился",
					slog.String("file", toRel(abs)),
					slog.Time("old_mod", cached.ModTime),
					slog.Time("new_mod", current.ModTime),
					slog.Int64("old_size", cached.Size),
					slog.Int64("new_size", current.Size),
				)
			}
			changed = append(changed, path)
		} else {
			slog.Debug("файл не изменился", slog.String("file", toRel(abs)))
		}
	}

	slog.Debug("проверка изменений", slog.Int("total", len(files)), slog.Int("changed", len(changed)))
	return changed, nil
}

var docExts = map[string]bool{".doc": true, ".docx": true, ".rtf": true}

// FilesToConvert возвращает список файлов из папок inDirs, которые нужно конвертировать в PDF.
//
// xmlFiles — конкретные пути к XML файлам с данными.
//
// Правила:
//   - outDir пуст → конвертировать всё из inDirs
//   - изменился любой XML из xmlFiles → конвертировать всё из inDirs
//   - изменились конкретные docx/rtf в inDirs → конвертировать только их
//   - ничего не изменилось → пустой список
func FilesToConvert(inDirs []string, xmlFiles []string, outDir string, withHash bool) ([]string, error) {
	c, err := LoadCache()
	if err != nil {
		return nil, fmt.Errorf("загрузить кэш: %w", err)
	}

	inFiles, err := collectFiles(inDirs, docExts)
	if err != nil {
		return nil, fmt.Errorf("собрать файлы In: %w", err)
	}
	if len(inFiles) == 0 {
		slog.Debug("папки In не содержат файлов для конвертации")
		return nil, nil
	}

	slog.Debug("найдено файлов в In", slog.Int("count", len(inFiles)))

	// Правило 1: outDir пуст — конвертируем всё.
	slog.Debug("правило 1: проверяем папку Out", slog.String("dir", outDir))
	empty, err := isDirEmpty(outDir)
	if err != nil {
		return nil, fmt.Errorf("проверить outDir: %w", err)
	}
	if empty {
		slog.Debug("папка Out пуста — конвертируем все файлы", slog.Int("count", len(inFiles)))
		return inFiles, nil
	}
	slog.Debug("правило 1: папка Out не пуста, проверяем XML")

	// Правило 2: изменился любой XML — конвертируем всё.
	slog.Debug("правило 2: проверяем XML файлы", slog.Int("count", len(xmlFiles)))
	xmlChanged, err := FilterChanged(xmlFiles, c, withHash)
	if err != nil {
		return nil, fmt.Errorf("проверить XML: %w", err)
	}
	if len(xmlChanged) > 0 {
		slog.Debug("изменились XML файлы — конвертируем все файлы",
			slog.Int("xml_changed", len(xmlChanged)),
			slog.Int("count", len(inFiles)),
		)
		return inFiles, nil
	}
	slog.Debug("правило 2: XML не изменились, проверяем шаблоны")

	// Правило 3: изменились конкретные docx/rtf — конвертируем только их.
	slog.Debug("правило 3: проверяем файлы шаблонов", slog.Int("count", len(inFiles)))
	changed, err := FilterChanged(inFiles, c, withHash)
	if err != nil {
		return nil, err
	}

	if len(changed) == 0 {
		slog.Debug("все файлы актуальны, конвертировать нечего")
	} else {
		slog.Debug("файлов к конвертации", slog.Int("count", len(changed)))
	}

	return changed, nil
}

func isChanged(cached, current FileEntry, withHash bool) bool {
	if withHash {
		return cached.Hash != current.Hash
	}
	return cached.ModTime.UnixNano() != current.ModTime.UnixNano() || cached.Size != current.Size
}

func makeEntry(path string, withHash bool) (FileEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileEntry{}, err
	}

	entry := FileEntry{
		ModTime: info.ModTime(),
		Size:    info.Size(),
	}

	if withHash {
		h, err := hashFile(path)
		if err != nil {
			return FileEntry{}, err
		}
		entry.Hash = h
	}

	return entry, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func collectFiles(dirs []string, exts map[string]bool) ([]string, error) {
	var files []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasPrefix(d.Name(), "~$") {
				return nil
			}
			ext := filepath.Ext(path)
			if len(exts) == 0 || exts[ext] {
				abs, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				files = append(files, abs)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func pruneCache(c Cache) {
	for relPath := range c {
		if _, err := os.Stat(toAbs(relPath)); os.IsNotExist(err) {
			slog.Debug("файл удалён, убираем из кэша", slog.String("file", relPath))
			delete(c, relPath)
		}
	}
}

// ConvCache — реализация convert.ConvCache (структурная типизация).
type ConvCache struct{}

// FilesToConvert делегирует в одноимённую функцию пакета.
func (ConvCache) FilesToConvert(inDirs []string, xmlFiles []string, outDir string, withHash bool) ([]string, error) {
	return FilesToConvert(inDirs, xmlFiles, outDir, withHash)
}

// CommitCache делегирует в одноимённую функцию пакета (возвращает только ошибку).
func (ConvCache) CommitCache(dirs []string, exts map[string]bool, withHash bool) error {
	_, err := CommitCache(dirs, exts, withHash)
	return err
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		slog.Debug("папка не существует, считаем пустой", slog.String("dir", dir))
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.IsDir() &&
			!strings.HasPrefix(e.Name(), "~$") &&
			strings.ToLower(filepath.Ext(e.Name())) == ".pdf" { // только PDF
			slog.Debug("папка не пуста", slog.String("dir", dir), slog.String("first_file", e.Name()))
			return false, nil
		}
	}
	slog.Debug("папка пуста (нет PDF файлов)", slog.String("dir", dir))
	return true, nil
}

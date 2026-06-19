package pdf

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nemo08/ppdftb/pkg/appendixutil"
	"github.com/maruel/natural"
)

func readDirNaturalSort(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return natural.Less(entries[i].Name(), entries[j].Name())
	})
	return entries, nil
}

// knownExts — реальные расширения документов/изображений.
// Файлы, чья часть после последней точки не входит в этот список,
// считаются не имеющими расширения (разделители).
var knownExts = map[string]bool{
	".pdf": true, ".docx": true, ".doc": true,
	".xlsx": true, ".xls": true,
	".pptx": true, ".ppt": true,
	".txt": true,
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".bmp": true, ".tiff": true, ".tif": true,
}

// ExtOf возвращает расширение файла, если оно из списка известных документов;
// иначе "" — файл считается без расширения (маркер-разделитель приложений).
func ExtOf(name string) string {
	return extOf(name)
}

func extOf(name string) string {
	e := strings.ToLower(filepath.Ext(name))
	if knownExts[e] {
		return e
	}
	return ""
}

func stripExt(name string) string {
	// Удаляем только если за точкой идёт известное расширение.
	if knownExts[strings.ToLower(filepath.Ext(name))] {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

func joinPath(dir, name string) string {
	return filepath.Join(dir, name)
}

// cleanFileName убирает порядковый номер в начале имени файла.
// Делегирует в appendixutil.CleanFileName.
func cleanFileName(base string) string {
	return appendixutil.CleanFileName(base)
}

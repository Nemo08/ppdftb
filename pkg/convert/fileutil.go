//go:build windows

package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readFiles читает все файлы из переданных путей и возвращает содержимое.
// Это общий хелпер для GetDataContent и FindXMLFiles.
func readFiles(paths []string) ([][]byte, error) {
	var result [][]byte
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			return result, err
		}
		result = append(result, content)
	}
	return result, nil
}

// GetDataContent читает файлы из source и возвращает их содержимое как [][]byte.
func GetDataContent(ctx context.Context, source []string) ([][]byte, error) {
	return readFiles(source)
}

// walkUpDirs собирает директории от startDir вверх на steps шагов,
// результат отранжирован от верхней к нижней (parent → child).
func walkUpDirs(startDir string, steps int) ([]string, error) {
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	dirs := make([]string, 0, steps+1)
	current := absDir
	for i := 0; i <= steps; i++ {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// reverse: parent first, child last
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs, nil
}

// FindXMLFiles поднимается вверх по ФС от startDir на steps шагов,
// собирает XML-файлы из каждой директории в алфавитном порядке
// и возвращает их содержимое от верхнего уровня к startDir.
func FindXMLFiles(startDir string, steps int) ([][]byte, []string, error) {
	dirs, err := walkUpDirs(startDir, steps)
	if err != nil {
		return nil, nil, err
	}

	var xmlPaths []string
	for _, dir := range dirs {
		files, err := CollectFiles([]string{dir}, []string{".xml"})
		if err != nil {
			return nil, xmlPaths, fmt.Errorf("failed to read dir %s: %w", dir, err)
		}
		xmlPaths = append(xmlPaths, files...)
	}

	data, err := readFiles(xmlPaths)
	if err != nil {
		return nil, xmlPaths, fmt.Errorf("failed to read XML: %w", err)
	}
	return data, xmlPaths, nil
}

// CollectFiles собирает файлы с указанными расширениями из списка источников.
func CollectFiles(sources []string, exts []string, skipPrefix ...string) ([]string, error) {
	var result []string
	for _, src := range sources {
		info, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("недоступен источник %q: %w", src, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(src)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if hasAnyPrefix(name, skipPrefix) {
					continue
				}
				ext := strings.ToLower(filepath.Ext(name))
				if !hasExt(ext, exts) {
					continue
				}
				abs, err := filepath.Abs(filepath.Join(src, name))
				if err != nil {
					return nil, err
				}
				result = append(result, abs)
			}
		} else {
			ext := strings.ToLower(filepath.Ext(src))
			if !hasExt(ext, exts) {
				continue
			}
			abs, err := filepath.Abs(src)
			if err != nil {
				return nil, err
			}
			result = append(result, abs)
		}
	}
	return result, nil
}

// CollectWordFiles собирает все *.doc/*.docx/*.rtf из списка файлов и папок.
func CollectWordFiles(sources []string) ([]string, error) {
	return CollectFiles(sources, []string{".doc", ".docx", ".rtf"}, "~$")
}

// CollectCadFiles собирает DWG/DXF из файла или папки.
func CollectCadFiles(sourceFile, sourceFolder string) ([]string, error) {
	var sources []string
	if sourceFile != "" {
		sources = append(sources, sourceFile)
	}
	if sourceFolder != "" {
		sources = append(sources, sourceFolder)
	}
	if len(sources) == 0 {
		return nil, nil
	}
	return CollectFiles(sources, []string{".dwg", ".dxf"})
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func hasExt(ext string, exts []string) bool {
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

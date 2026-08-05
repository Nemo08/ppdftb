package fileutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

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

func GetDataContent(ctx context.Context, source []string) ([][]byte, error) {
	return readFiles(source)
}

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

	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs, nil
}

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

func CollectFiles(sources []string, exts []string, skipPrefix ...string) ([]string, error) {
	var result []string
	for _, src := range sources {
		info, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("недоступен источник %q: %w", src, err)
		}
		if info.IsDir() {
			files, err := collectFromDir(src, exts, skipPrefix)
			if err != nil {
				return nil, err
			}
			result = append(result, files...)
			continue
		}
		file, err := collectFromFile(src, exts)
		if err != nil {
			return nil, err
		}
		if file != "" {
			result = append(result, file)
		}
	}
	return result, nil
}

// collectFromDir собирает файлы заданных расширений из директории,
// пропуская вложенные папки и файлы с указанными префиксами.
func collectFromDir(dir string, exts []string, skipPrefix []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []string
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
		abs, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		result = append(result, abs)
	}
	return result, nil
}

// collectFromFile возвращает абсолютный путь файла, если его расширение
// входит в exts, иначе пустую строку.
func collectFromFile(src string, exts []string) (string, error) {
	ext := strings.ToLower(filepath.Ext(src))
	if !hasExt(ext, exts) {
		return "", nil
	}
	return filepath.Abs(src)
}

func CollectWordFiles(sources []string) ([]string, error) {
	return CollectFiles(sources, []string{".doc", ".docx", ".rtf"}, "~$")
}

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
	return slices.Contains(exts, ext)
}

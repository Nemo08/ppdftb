package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CollectFiles собирает файлы с указанными расширениями из списка источников.
// sources — файлы и/или папки. skipPrefix — префиксы для пропуска (например "~$").
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

// DataMerge объединяет несколько XML-документов в один JSON.
// Первый документ — база; каждый следующий перезаписывает/добавляет поля.
// Используется для слияния XML-данных из нескольких источников перед
// подстановкой в шаблон DOCX.
func DataMerge(data [][]byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("no data provided")
	}

	merged, err := xmlToMap(bytes.NewReader(data[0]))
	if err != nil {
		return nil, fmt.Errorf("file 1: %w", err)
	}

	for i, d := range data[1:] {
		m, err := xmlToMap(bytes.NewReader(d))
		if err != nil {
			return nil, fmt.Errorf("file %d: %w", i+2, err)
		}
		mergeMaps(merged, m)
	}

	return json.MarshalIndent(merged, "", "  ")
}

// xmlToMap разбирает XML в map[string]any.
// Корневой тег отбрасывается, его дети становятся ключами карты.
func xmlToMap(r io.Reader) (map[string]any, error) {
	dec := xml.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	start, ok := tok.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("expected root element")
	}

	// Корень — всегда карта (не коллапсим).
	v, err := readValue(dec, start, false)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root must be an object")
	}
	return m, nil
}

// readValue рекурсивно читает XML-элемент и возвращает его значение:
// строка для листовых узлов, map[string]any для узлов с детьми.
func readValue(dec *xml.Decoder, start xml.StartElement, collapse bool) (any, error) {
	children := make(map[string]any)
	var text string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			if len(children) > 0 {
				if len(text) > 0 {
					children["#text"] = strings.TrimSpace(text)
				}
				if collapse {
					return collapseChildren(children), nil
				}
				return children, nil
			}
			return strings.TrimSpace(text), nil
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, err := readValue(dec, t, true) // дети всегда коллапсим
			if err != nil {
				return nil, err
			}
			key := t.Name.Local
			if existing, ok := children[key]; ok {
				switch v := existing.(type) {
				case []any:
					children[key] = append(v, child)
				default:
					children[key] = []any{v, child}
				}
			} else {
				children[key] = child
			}

		case xml.EndElement:
			if len(children) > 0 {
				if collapse {
					return collapseChildren(children), nil
				}
				return children, nil
			}
			return strings.TrimSpace(text), nil

		case xml.CharData:
			text += string(t)
		}
	}
}

// collapseChildren преобразует карту вида {"Param": [...]} в массив [...],
// если все дочерние элементы имеют один и тот же ключ (и >1 элемент, либо
// единственный элемент — массив). Иначе возвращает карту как есть.
func collapseChildren(children map[string]any) any {
	if len(children) == 0 {
		return children
	}

	if len(children) == 1 {
		for _, v := range children {
			// Одиночный элемент: если значение уже массив, отдаём его напрямую.
			if _, ok := v.([]any); ok {
				return v
			}
		}
		return children
	}

	// Если все ключи одинаковые — коллапсим.
	firstKey := ""
	allSame := true
	for k := range children {
		if firstKey == "" {
			firstKey = k
		} else if k != firstKey {
			allSame = false
			break
		}
	}

	if !allSame {
		return children
	}

	// Все ключи одинаковые. Если за этим ключом массив — отдаём его.
	if v, ok := children[firstKey]; ok {
		return v
	}
	return children
}

// mergeMaps рекурсивно сливает src в dst (src перезаписывает dst при совпадении ключей).
func mergeMaps(dst, src map[string]any) {
	for k, sv := range src {
		dv, ok := dst[k]
		if !ok {
			dst[k] = sv
			continue
		}
		dmap, dok := dv.(map[string]any)
		smap, sok := sv.(map[string]any)
		if dok && sok {
			mergeMaps(dmap, smap)
		} else {
			dst[k] = sv
		}
	}
}

// GetDataContent читает файлы из source и возвращает их содержимое как [][]byte.
// Каждый файл читается полностью; ошибка чтения любого файла прерывает весь процесс.
func GetDataContent(ctx context.Context, source []string) ([][]byte, error) {
	var content []byte
	var result [][]byte
	var err error

	for _, path := range source {
		content, err = os.ReadFile(path)
		if err != nil {
			return result, err
		}
		result = append(result, content)
	}
	return result, nil
}

// FindXMLFiles поднимается вверх по ФС от startDir на steps шагов,
// собирает XML-файлы из каждой директории в алфавитном порядке
// и возвращает их содержимое от верхнего уровня к startDir.
func FindXMLFiles(startDir string, steps int) ([][]byte, []string, error) {
	var xmlPaths []string
	var result [][]byte
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, xmlPaths, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Идём вверх, собирая директории
	dirs := make([]string, 0, steps+1)
	current := absDir
	for i := 0; i <= steps; i++ {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break // корень ФС
		}
		current = parent
	}

	// Разворачиваем: верхний уровень — первым
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	for _, dir := range dirs {
		files, err := CollectFiles([]string{dir}, []string{".xml"})
		if err != nil {
			return nil, xmlPaths, fmt.Errorf("failed to read dir %s: %w", dir, err)
		}

		sort.Strings(files)

		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, xmlPaths, fmt.Errorf("failed to read file %s: %w", path, err)
			}
			result = append(result, data)
			xmlPaths = append(xmlPaths, path)
		}
	}

	return result, xmlPaths, nil
}

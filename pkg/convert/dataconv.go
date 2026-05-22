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
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, xmlPaths, fmt.Errorf("failed to read dir %s: %w", dir, err)
		}

		// Фильтруем только XML-файлы
		var xmlNames []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.ToLower(filepath.Ext(entry.Name())) == ".xml" {
				xmlNames = append(xmlNames, entry.Name())
			}
		}

		// Явная сортировка по алфавиту
		sort.Strings(xmlNames)

		for _, name := range xmlNames {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, xmlPaths, fmt.Errorf("failed to read file %s: %w", name, err)
			}
			result = append(result, data)
			xmlPaths = append(xmlPaths, filepath.Join(dir, name))
		}
	}

	return result, xmlPaths, nil
}

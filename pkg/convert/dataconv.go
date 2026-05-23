package convert

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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
			child, err := readValue(dec, t, true)
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
			if _, ok := v.([]any); ok {
				return v
			}
		}
		return children
	}

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

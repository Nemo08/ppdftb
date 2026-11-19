package pdfpl

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ValuesFromXML разбирает XML-данные в плоскую карту «имя листового элемента →
// текстовое значение». Используется как связующее звено между мерджнутыми XML
// (формат wconv) и Values в Replace/ReplaceFile: ключи совпадают с именами
// листовых элементов (например DeveloperOrgFullName), что соответствует
// плейсхолдерам {{.DeveloperOrgFullName}}.
//
// Вложенные объекты раскрываются рекурсивно; при повторяющихся ключах
// (массивы) в карту попадает первое вхождение. Текстовые значения обрезаются
// по краям.
func ValuesFromXML(r io.Reader) (map[string]string, error) {
	dec := xml.NewDecoder(r)
	root, err := nextElementStart(dec)
	if err != nil {
		return nil, err
	}
	return collectValues(dec, root.Name.Local)
}

// nextElementStart возвращает первый StartElement документа (корень).
func nextElementStart(dec *xml.Decoder) (*xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("pdfpl: чтение корневого элемента: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return &se, nil
		}
	}
}

// collectValues рекурсивно собирает листовые текстовые значения под элементом parent.
func collectValues(dec *xml.Decoder, parent string) (map[string]string, error) {
	values := make(map[string]string)
	var text strings.Builder

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("pdfpl: разбор < %s >: %w", parent, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, err := collectValues(dec, t.Name.Local)
			if err != nil {
				return nil, err
			}
			mergeFirst(values, child)

		case xml.EndElement:
			if t.Name.Local == parent {
				if s := strings.TrimSpace(text.String()); s != "" {
					values[parent] = s
				}
				return values, nil
			}

		case xml.CharData:
			text.WriteString(string(t))
		}
	}
}

// mergeFirst добавляет в dst отсутствующие ключи из src (переживают первые значения).
func mergeFirst(dst, src map[string]string) {
	for k, v := range src {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
}

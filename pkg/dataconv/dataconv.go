package dataconv

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func DataMerge(data [][]byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("no data provided")
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

	postProcessMerge(merged)
	return json.MarshalIndent(merged, "", "  ")
}

// postProcessMerge выполняет пост-обработку объединённых данных:
// — собирает Actsigners из плоских полей ActSigner1* / ActSigner2*
//
//	(шаблоны ожидают массив подписантов, а XML содержит отдельные поля)
func postProcessMerge(data map[string]any) {
	// Сборка Actsigners
	var signers []any
	for i := 1; ; i++ {
		posKey := fmt.Sprintf("ActSigner%dPosition", i)
		fioKey := fmt.Sprintf("ActSigner%dFIO", i)
		pos, posOk := data[posKey].(string)
		fio, fioOk := data[fioKey].(string)
		if !posOk && !fioOk {
			break
		}
		sign := ""
		if fio != "" {
			if parts := strings.SplitN(fio, " ", 2); len(parts) > 0 {
				sign = parts[0] + ".png"
			}
		}
		signer := map[string]any{
			"Position": pos,
			"FIO":      fio,
			"Sign":     sign,
		}
		signers = append(signers, signer)
	}
	if len(signers) > 0 {
		data["Actsigners"] = signers
	}
}

func xmlToMap(r io.Reader) (map[string]any, error) {
	dec := xml.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	start, ok := tok.(xml.StartElement)
	if !ok {
		return nil, errors.New("expected root element")
	}

	v, err := readValue(dec, start, false)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("root must be an object")
	}
	return m, nil
}

func readValue(dec *xml.Decoder, start xml.StartElement, collapse bool) (any, error) {
	children := make(map[string]any)
	var text string

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return readValueEOF(children, text, collapse)
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if err := readStartElement(dec, t, children); err != nil {
				return nil, err
			}

		case xml.EndElement:
			return readValueEnd(children, text, collapse)

		case xml.CharData:
			text += string(t)
		}
	}
}

func readValueEOF(children map[string]any, text string, collapse bool) (any, error) {
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

func readValueEnd(children map[string]any, text string, collapse bool) (any, error) {
	if len(children) > 0 {
		if collapse {
			return collapseChildren(children), nil
		}
		return children, nil
	}
	return strings.TrimSpace(text), nil
}

func readStartElement(dec *xml.Decoder, t xml.StartElement, children map[string]any) error {
	child, err := readValue(dec, t, true)
	if err != nil {
		return err
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
	return nil
}

func collapseChildren(children map[string]any) any {
	if len(children) == 0 {
		return children
	}

	if len(children) == 1 {
		return collapseSingleChild(children)
	}

	firstKey, allSame := singleKeyOf(children)
	if !allSame {
		return children
	}

	if v, ok := children[firstKey]; ok {
		return v
	}
	return children
}

// collapseSingleChild сворачивает единственного потомка: список остаётся
// списком, одиночная карта оборачивается в список из одного элемента.
func collapseSingleChild(children map[string]any) any {
	for _, v := range children {
		switch v.(type) {
		case []any:
			return v
		case map[string]any:
			return []any{v}
		}
	}
	return children
}

// singleKeyOf возвращает первый ключ и признак того, что все ключи одинаковы.
func singleKeyOf(children map[string]any) (string, bool) {
	firstKey := ""
	for k := range children {
		if firstKey == "" {
			firstKey = k
		} else if k != firstKey {
			return firstKey, false
		}
	}
	return firstKey, true
}

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

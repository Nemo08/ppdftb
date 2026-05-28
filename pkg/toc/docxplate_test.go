package toc

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	gotemplatedocx "github.com/JJJJJJack/go-template-docx"
)

// buildMinimalDocx creates a minimal valid DOCX with the given document body.
func buildMinimalDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	ct, _ := w.Create("[Content_Types].xml")
	_, _ = ct.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))

	rels, _ := w.Create("word/_rels/document.xml.rels")
	_, _ = rels.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`))

	doc, _ := w.Create("word/document.xml")
	_, _ = doc.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:tbl>
      <w:tr>
        <w:tc>
          <w:p>
            <w:r><w:t>{{Pages.Name}}</w:t><w:t>{{Pages.Page}}</w:t></w:r>
          </w:p>
        </w:tc>
      </w:tr>
    </w:tbl>
    <w:sectPr>
      <w:pgSz w:w="12240" w:h="15840"/>
      <w:pgMar w:top="1440" w:bottom="1440" w:left="1440" w:right="1440"/>
    </w:sectPr>
  </w:body>
</w:document>`))
	_ = w.Close()
	return buf.Bytes()
}

// buildFragmentedDocx создаёт DOCX, где {{ и }} разбиты по разным <w:r>.
// Word разбивает { и { на разные runs с <w:proofErr> между ними.
func buildFragmentedDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	ct, _ := w.Create("[Content_Types].xml")
	_, _ = ct.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))

	rels, _ := w.Create("word/_rels/document.xml.rels")
	_, _ = rels.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`))

	doc, _ := w.Create("word/document.xml")
	_, _ = doc.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:tbl>
      <w:tr>
        <w:tc>
          <w:p>
            <w:r><w:t>{</w:t></w:r>
            <w:proofErr w:type="spellStart"/>
            <w:r><w:t>{Pages.Name}</w:t></w:r>
            <w:proofErr w:type="spellEnd"/>
            <w:r><w:t>}</w:t></w:r>
          </w:p>
        </w:tc>
      </w:tr>
      <w:tr>
        <w:tc>
          <w:p>
            <w:r><w:t>{</w:t></w:r>
            <w:proofErr w:type="spellStart"/>
            <w:r><w:t>{Pages.Page}</w:t></w:r>
            <w:proofErr w:type="spellEnd"/>
            <w:r><w:t>}</w:t></w:r>
          </w:p>
        </w:tc>
      </w:tr>
    </w:tbl>
    <w:sectPr>
      <w:pgSz w:w="12240" w:h="15840"/>
      <w:pgMar w:top="1440" w:bottom="1440" w:left="1440" w:right="1440"/>
    </w:sectPr>
  </w:body>
</w:document>`))
	_ = w.Close()
	return buf.Bytes()
}

// extractDocumentXML извлекает word/document.xml из DOCX (ZIP) для проверок.
func extractDocumentXML(t *testing.T, docxBytes []byte) string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(rc)
			_ = rc.Close()
			return buf.String()
		}
	}
	t.Fatal("word/document.xml not found in result")
	return ""
}

func TestPatchXmlBeforeAutoExpand_NonFragmented(t *testing.T) {
	// Нефрагментированный шаблон — базовая проверка автоэкспанда после удаления workaround.
	docxBytes := buildMinimalDocx(t, "")
	data := map[string]any{
		"Pages": []any{
			map[string]any{"Name": "Раздел 1", "Page": "5"},
			map[string]any{"Name": "Раздел 2", "Page": "10"},
		},
	}

	result, err := gotemplatedocx.Render(docxBytes, data,
		gotemplatedocx.WithDeleteMissingKey(),
		gotemplatedocx.WithAutoExpandRows(data),
	)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	xml := extractDocumentXML(t, result)
	// 2 элемента → 2 <w:tr> (исходная строка + 1 клон)
	if count := strings.Count(xml, "<w:tr>"); count != 2 {
		t.Errorf("expected 2 <w:tr> for 2 entries, got %d\nXML:\n%s", count, xml)
	}
}

func TestPatchXmlBeforeAutoExpand_FragmentedTemplate(t *testing.T) {
	// Если PatchXML не выполнился до AutoExpandRows, то {{Pages.Name}}
	// разбитый по runs не будет распознан.
	// С фиксом библиотеки PatchXML бежит как DefaultPreProcessor,
	// поэтому даже фрагментированный шаблон отрабатывает.
	docxBytes := buildFragmentedDocx(t)

	data := map[string]any{
		"Pages": []any{
			map[string]any{"Name": "Раздел 1", "Page": "5"},
			map[string]any{"Name": "Раздел 2", "Page": "10"},
		},
	}

	result, err := gotemplatedocx.Render(docxBytes, data,
		gotemplatedocx.WithDeleteMissingKey(),
		gotemplatedocx.WithAutoExpandRows(data),
	)
	if err != nil {
		t.Fatalf("Render failed (PatchXML likely not running before AutoExpandRows): %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	xml := extractDocumentXML(t, result)
	// 2 элемента → 2 строки, но в исходном DOCX уже 2 строки (по одной на Pages.Name и Pages.Page),
	// автоэкспанд каждую размножит до 2 → итого 4 строки.
	// Каждая исходная строка содержит один шаблон, Pages - массив из 2 → каждая даёт 2 строки.
	if count := strings.Count(xml, "<w:tr>"); count != 4 {
		t.Errorf("expected 4 <w:tr> (2 rows × 2 items), got %d\nXML:\n%s", count, xml)
	}
}

func TestPatchXmlBeforeAutoExpand_ThreeItems(t *testing.T) {
	docxBytes := buildMinimalDocx(t, "")
	data := map[string]any{
		"Pages": []any{
			map[string]any{"Name": "A", "Page": "1"},
			map[string]any{"Name": "B", "Page": "2"},
			map[string]any{"Name": "C", "Page": "3"},
		},
	}

	result, err := gotemplatedocx.Render(docxBytes, data,
		gotemplatedocx.WithDeleteMissingKey(),
		gotemplatedocx.WithAutoExpandRows(data),
	)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	xml := extractDocumentXML(t, result)
	if count := strings.Count(xml, "<w:tr>"); count != 3 {
		t.Errorf("expected 3 <w:tr> for 3 items, got %d\nXML:\n%s", count, xml)
	}
}

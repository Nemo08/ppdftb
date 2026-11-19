package toc

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	gotemplatedocx "github.com/JJJJJJack/go-template-docx"
)

// buildMinimalDocx creates a minimal valid DOCX with a fixed document body.
func buildMinimalDocx(t *testing.T) []byte {
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

// buildFragmentedDocx СЃРѕР·РґР°С‘С‚ DOCX, РіРґРµ {{ Рё }} СЂР°Р·Р±РёС‚С‹ РїРѕ СЂР°Р·РЅС‹Рј <w:r>.
// Word СЂР°Р·Р±РёРІР°РµС‚ { Рё { РЅР° СЂР°Р·РЅС‹Рµ runs СЃ <w:proofErr> РјРµР¶РґСѓ РЅРёРјРё.
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

// extractDocumentXML РёР·РІР»РµРєР°РµС‚ word/document.xml РёР· DOCX (ZIP) РґР»СЏ РїСЂРѕРІРµСЂРѕРє.
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
	// РќРµС„СЂР°РіРјРµРЅС‚РёСЂРѕРІР°РЅРЅС‹Р№ С€Р°Р±Р»РѕРЅ вЂ” Р±Р°Р·РѕРІР°СЏ РїСЂРѕРІРµСЂРєР° Р°РІС‚РѕСЌРєСЃРїР°РЅРґР° РїРѕСЃР»Рµ СѓРґР°Р»РµРЅРёСЏ workaround.
	docxBytes := buildMinimalDocx(t)
	data := map[string]any{
		"Pages": []any{
			map[string]any{"Name": "Р Р°Р·РґРµР» 1", "Page": "5"},
			map[string]any{"Name": "Р Р°Р·РґРµР» 2", "Page": "10"},
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
	// 2 СЌР»РµРјРµРЅС‚Р° в†’ 2 <w:tr> (РёСЃС…РѕРґРЅР°СЏ СЃС‚СЂРѕРєР° + 1 РєР»РѕРЅ)
	if count := strings.Count(xml, "<w:tr>"); count != 2 {
		t.Errorf("expected 2 <w:tr> for 2 entries, got %d\nXML:\n%s", count, xml)
	}
}

func TestPatchXmlBeforeAutoExpand_FragmentedTemplate(t *testing.T) {
	// Р•СЃР»Рё PatchXML РЅРµ РІС‹РїРѕР»РЅРёР»СЃСЏ РґРѕ AutoExpandRows, С‚Рѕ {{Pages.Name}}
	// СЂР°Р·Р±РёС‚С‹Р№ РїРѕ runs РЅРµ Р±СѓРґРµС‚ СЂР°СЃРїРѕР·РЅР°РЅ.
	// РЎ С„РёРєСЃРѕРј Р±РёР±Р»РёРѕС‚РµРєРё PatchXML Р±РµР¶РёС‚ РєР°Рє DefaultPreProcessor,
	// РїРѕСЌС‚РѕРјСѓ РґР°Р¶Рµ С„СЂР°РіРјРµРЅС‚РёСЂРѕРІР°РЅРЅС‹Р№ С€Р°Р±Р»РѕРЅ РѕС‚СЂР°Р±Р°С‚С‹РІР°РµС‚.
	docxBytes := buildFragmentedDocx(t)

	data := map[string]any{
		"Pages": []any{
			map[string]any{"Name": "Р Р°Р·РґРµР» 1", "Page": "5"},
			map[string]any{"Name": "Р Р°Р·РґРµР» 2", "Page": "10"},
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
	// 2 СЌР»РµРјРµРЅС‚Р° в†’ 2 СЃС‚СЂРѕРєРё, РЅРѕ РІ РёСЃС…РѕРґРЅРѕРј DOCX СѓР¶Рµ 2 СЃС‚СЂРѕРєРё (РїРѕ РѕРґРЅРѕР№ РЅР° Pages.Name Рё Pages.Page),
	// Р°РІС‚РѕСЌРєСЃРїР°РЅРґ РєР°Р¶РґСѓСЋ СЂР°Р·РјРЅРѕР¶РёС‚ РґРѕ 2 в†’ РёС‚РѕРіРѕ 4 СЃС‚СЂРѕРєРё.
	// РљР°Р¶РґР°СЏ РёСЃС…РѕРґРЅР°СЏ СЃС‚СЂРѕРєР° СЃРѕРґРµСЂР¶РёС‚ РѕРґРёРЅ С€Р°Р±Р»РѕРЅ, Pages - РјР°СЃСЃРёРІ РёР· 2 в†’ РєР°Р¶РґР°СЏ РґР°С‘С‚ 2 СЃС‚СЂРѕРєРё.
	if count := strings.Count(xml, "<w:tr>"); count != 4 {
		t.Errorf("expected 4 <w:tr> (2 rows Г— 2 items), got %d\nXML:\n%s", count, xml)
	}
}

func TestPatchXmlBeforeAutoExpand_ThreeItems(t *testing.T) {
	docxBytes := buildMinimalDocx(t)
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

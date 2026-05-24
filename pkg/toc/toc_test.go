package toc

import (
	"testing"
)

func TestExtractCleanName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"with space separator", "AB-1234-CDE Проект здания", "Проект здания"},
		{"no space", "SomeName", "SomeName"},
		{"empty", "", ""},
		{"trailing space", "AB-12  Имя", "Имя"},
		{"only prefix", "Prefix", "Prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCleanName(tt.in)
			if got != tt.want {
				t.Errorf("extractCleanName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildTemplateData(t *testing.T) {
	files := []OnePDFFile{
		{cleanName: "Проект", pages: 10},
		{cleanName: "Пояснилка", pages: 5},
		{cleanName: "Спецификация", pages: 3},
	}
	td := buildTemplateData(files, 3)
	if td.Number != 0 {
		t.Errorf("Number = %d, want 0", td.Number)
	}
	if len(td.Pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(td.Pages))
	}
	want := []struct {
		name string
		page int
	}{
		{"Проект", 3},
		{"Пояснилка", 13},
		{"Спецификация", 18},
	}
	for i, w := range want {
		if td.Pages[i].Name != w.name || td.Pages[i].Page != w.page {
			t.Errorf("Page[%d] = %+v, want {Name: %q, Page: %d}", i, td.Pages[i], w.name, w.page)
		}
	}
}

func TestBuildTemplateDataEmpty(t *testing.T) {
	td := buildTemplateData(nil, 1)
	if len(td.Pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(td.Pages))
	}
}

func TestBuildPdfFileListEmpty(t *testing.T) {
	result := buildPdfFileList(nil, "pdn", "template.docx", nil)
	if len(result) != 0 {
		t.Errorf("expected 0 files, got %d", len(result))
	}
}

func TestBuildPdfFileList(t *testing.T) {
	pdfList := []string{
		"Содержание.pdf",
		"01-01-ABC Первый документ.pdf",
		"01-02-DEF Второй документ.pdf",
	}
	pdn := "test_pdf"
	tfn := "test_pdf/Содержание.docx"
	pageCounts := map[string]int{
		"Содержание.pdf":                1,
		"01-01-ABC Первый документ.pdf": 10,
		"01-02-DEF Второй документ.pdf": 5,
	}

	result := buildPdfFileList(pdfList, pdn, tfn, pageCounts)
	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(result), result)
	}
	if result[0].pages != 1 {
		t.Errorf("result[0].pages = %d, want 1", result[0].pages)
	}
	if result[1].cleanName != "Первый документ" {
		t.Errorf("result[1].cleanName = %q, want %q", result[1].cleanName, "Первый документ")
	}
	if result[1].pages != 10 {
		t.Errorf("result[1].pages = %d, want 10", result[1].pages)
	}
	if result[2].cleanName != "Второй документ" {
		t.Errorf("result[2].cleanName = %q, want %q", result[2].cleanName, "Второй документ")
	}
	if result[2].pages != 5 {
		t.Errorf("result[2].pages = %d, want 5", result[2].pages)
	}
}

func TestBuildPdfFileListAddOnMarker(t *testing.T) {
	pdfList := []string{
		"toc.pdf",
		"00-cover.pdf",
		"01-project.pdf",
	}
	pdn := "pdn"
	tfn := "pdn/toc.docx"

	result := buildPdfFileList(pdfList, pdn, tfn, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result))
	}
	if result[0].fileName != "toc.pdf" {
		t.Errorf("first file = %v, want toc.pdf", result[0].fileName)
	}
}

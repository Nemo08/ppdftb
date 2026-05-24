package toc

import "testing"

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

func TestSplitFileBase(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantObozn string
		wantName  string
	}{
		{"standard", "01-01-ABC Проект здания", "01-01-ABC", "Проект здания"},
		{"no space", "SomeName", "SomeName", "SomeName"},
		{"empty", "", "", ""},
		{"multi space", "AB-12  Имя файла", "AB-12", "Имя файла"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obozn, name := splitFileBase(tt.in)
			if obozn != tt.wantObozn {
				t.Errorf("splitFileBase(%q) obozn = %q, want %q", tt.in, obozn, tt.wantObozn)
			}
			if name != tt.wantName {
				t.Errorf("splitFileBase(%q) name = %q, want %q", tt.in, name, tt.wantName)
			}
		})
	}
}

func TestBuildTemplateData(t *testing.T) {
	files := []OnePDFFile{
		{obozn: "01", cleanName: "Проект", pages: 10},
		{obozn: "02", cleanName: "Пояснилка", pages: 5},
		{obozn: "03", cleanName: "Спецификация", pages: 3},
	}
	td := buildTemplateData(files, 3)
	if td.Number != 0 {
		t.Errorf("Number = %d, want 0", td.Number)
	}
	if len(td.Pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(td.Pages))
	}
	want := []struct {
		obozn string
		name  string
		page  int
	}{
		{"01", "Проект", 3},
		{"02", "Пояснилка", 13},
		{"03", "Спецификация", 18},
	}
	for i, w := range want {
		if td.Pages[i].Obozn != w.obozn || td.Pages[i].Name != w.name || td.Pages[i].Page != w.page {
			t.Errorf("Page[%d] = %+v, want {Obozn: %q, Name: %q, Page: %d}", i, td.Pages[i], w.obozn, w.name, w.page)
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
	// Шаблон "Содержание.pdf" исключён, остаются 2 файла
	if len(result) != 2 {
		t.Fatalf("expected 2 files (template excluded), got %d: %+v", len(result), result)
	}
	if result[0].obozn != "01-01-ABC" {
		t.Errorf("result[0].obozn = %q, want %q", result[0].obozn, "01-01-ABC")
	}
	if result[0].cleanName != "Первый документ" {
		t.Errorf("result[0].cleanName = %q, want %q", result[0].cleanName, "Первый документ")
	}
	if result[0].pages != 10 {
		t.Errorf("result[0].pages = %d, want 10", result[0].pages)
	}
	if result[1].obozn != "01-02-DEF" {
		t.Errorf("result[1].obozn = %q, want %q", result[1].obozn, "01-02-DEF")
	}
	if result[1].cleanName != "Второй документ" {
		t.Errorf("result[1].cleanName = %q, want %q", result[1].cleanName, "Второй документ")
	}
	if result[1].pages != 5 {
		t.Errorf("result[1].pages = %d, want 5", result[1].pages)
	}
}

func TestBuildPdfFileListSkipsTemplate(t *testing.T) {
	pdfList := []string{
		"toc.pdf",
		"00-cover.pdf",
		"01-project.pdf",
	}
	pdn := "pdn"
	tfn := "pdn/toc.docx"

	result := buildPdfFileList(pdfList, pdn, tfn, nil)
	// Шаблон "toc.pdf" исключён, остаются 2 файла
	if len(result) != 2 {
		t.Fatalf("expected 2 files (template excluded), got %d", len(result))
	}
	if result[0].fileName != "00-cover.pdf" {
		t.Errorf("first file = %v, want 00-cover.pdf", result[0].fileName)
	}
	if result[1].fileName != "01-project.pdf" {
		t.Errorf("second file = %v, want 01-project.pdf", result[1].fileName)
	}
}

func TestBuildPdfFileListTemplateAtEnd(t *testing.T) {
	pdfList := []string{
		"00-cover.pdf",
		"01-project.pdf",
		"toc.pdf",
	}
	pdn := "pdn"
	tfn := "pdn/toc.docx"

	result := buildPdfFileList(pdfList, pdn, tfn, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 files (template excluded regardless of position), got %d", len(result))
	}
}

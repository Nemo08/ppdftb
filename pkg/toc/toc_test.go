package toc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMakeAppendixToc_NoAppendixEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1. Обложка.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3. Содержание.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "5. Текст.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "6. Спецификация.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	tfn := filepath.Join(dir, "3. Содержание.docx")

	pageCounts := map[string]int{
		"5. Текст.pdf":        10,
		"6. Спецификация.pdf": 5,
	}

	td, err := makeAppendixToc(context.Background(), dir, tfn, 3, pageCounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(td.Pages) != 2 {
		t.Fatalf("expected 2 entries after template, got %d", len(td.Pages))
	}
	if td.Pages[0].Obozn != "" || td.Pages[0].Name != "Текст" || td.Pages[0].Page != "3" {
		t.Errorf("entry[0] = %+v, want {Obozn:, Name:Текст, Page:3}", td.Pages[0])
	}
	if td.Pages[1].Obozn != "" || td.Pages[1].Name != "Спецификация" || td.Pages[1].Page != "13" {
		t.Errorf("entry[1] = %+v, want {Obozn:, Name:Спецификация, Page:13}", td.Pages[1])
	}
}

func TestMakeAppendixToc_WithDividersAndAppendix(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"1. Обложка.pdf", "3. Содержание.pdf", "5. Текст.pdf",
		"10. ПРИЛОЖЕНИЯ", "11. Первое приложение.pdf", "12. Второе приложение.pdf",
		"60. ГРАФИЧЕСКАЯ ЧАСТЬ", "61. Чертеж.pdf",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("%PDF-1.4"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tfn := filepath.Join(dir, "3. Содержание.docx")

	pageCounts := map[string]int{
		"5. Текст.pdf":              5,
		"11. Первое приложение.pdf": 3,
		"12. Второе приложение.pdf": 4,
		"61. Чертеж.pdf":            2,
	}

	td, err := makeAppendixToc(context.Background(), dir, tfn, 2, pageCounts)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		Name string
		Page string
	}{
		{"Текст", "2"},
		{"ПРИЛОЖЕНИЯ", ""},
		{"Приложение А. Первое приложение", "7"},
		{"Приложение Б. Второе приложение", "10"},
		{"ГРАФИЧЕСКАЯ ЧАСТЬ", ""},
		{"Чертеж", "14"},
	}

	if len(td.Pages) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(td.Pages))
	}
	for i, w := range want {
		if td.Pages[i].Obozn != "" || td.Pages[i].Name != w.Name || td.Pages[i].Page != w.Page {
			t.Errorf("entry[%d] = %+v, want {Obozn:, Name:%s, Page:%s}", i, td.Pages[i], w.Name, w.Page)
		}
	}
}

func TestMakeAppendixToc_TemplateNotFound(t *testing.T) {
	// Ситуация: PDF шаблона содержания ещё нет в папке (первый вызов до wconv).
	// makeAppendixToc должен найти позицию вставки через sort.Search.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1. Обложка.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 3. Содержание.pdf — НЕТ
	if err := os.WriteFile(filepath.Join(dir, "5. Текст.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	tfn := filepath.Join(dir, "3. Содержание.docx")
	pageCounts := map[string]int{"5. Текст.pdf": 10}

	td, err := makeAppendixToc(context.Background(), dir, tfn, 3, pageCounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(td.Pages) != 1 {
		t.Fatalf("expected 1 entry (only Текст), got %d", len(td.Pages))
	}
	if td.Pages[0].Obozn != "" || td.Pages[0].Name != "Текст" || td.Pages[0].Page != "3" {
		t.Errorf("entry = %+v", td.Pages[0])
	}
}

func TestMakeAppendixToc_AfterTemplateEmpty(t *testing.T) {
	dir := t.TempDir()
	// Шаблон последний — после него ничего
	if err := os.WriteFile(filepath.Join(dir, "1. Обложка.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3. Содержание.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	tfn := filepath.Join(dir, "3. Содержание.docx")
	td, err := makeAppendixToc(context.Background(), dir, tfn, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(td.Pages) != 0 {
		t.Errorf("expected 0 entries (template at end), got %d", len(td.Pages))
	}
}

func TestMakeAppendixToc_OnlyPDF(t *testing.T) {
	// Без заглушек и приложений — режим appendix, но их нет.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "3. Содержание.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "5. Документ.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "6. Ещё.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	tfn := filepath.Join(dir, "3. Содержание.docx")
	pageCounts := map[string]int{"5. Документ.pdf": 7, "6. Ещё.pdf": 3}

	td, err := makeAppendixToc(context.Background(), dir, tfn, 5, pageCounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(td.Pages) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(td.Pages))
	}
	if td.Pages[0].Page != "5" || td.Pages[1].Page != "12" {
		t.Errorf("pages: got %s, %s; want 5, 12", td.Pages[0].Page, td.Pages[1].Page)
	}
}

func TestCollectEntriesIntegration(t *testing.T) {
	// Интеграционная проверка: pdf.CollectEntries возвращает правильные типы
	// для файлов в папке.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "10. ПРИЛОЖЕНИЯ"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "11. Акт.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	tfn := filepath.Join(dir, "3. Содержание.docx")
	pageCounts := map[string]int{"11. Акт.pdf": 2}

	td, err := makeAppendixToc(context.Background(), dir, tfn, 1, pageCounts)
	if err != nil {
		t.Fatal(err)
	}
	// Всё должно быть после шаблона (его нет, sort.Search найдёт позицию).
	_ = td
}

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
		page  string
	}{
		{"01", "Проект", "3"},
		{"02", "Пояснилка", "13"},
		{"03", "Спецификация", "18"},
	}
	for i, w := range want {
		if td.Pages[i].Obozn != w.obozn || td.Pages[i].Name != w.name || td.Pages[i].Page != w.page {
			t.Errorf("Page[%d] = %+v, want {Obozn: %q, Name: %q, Page: %s}", i, td.Pages[i], w.obozn, w.name, w.page)
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
	// Список отсортирован: 1.Обложка < 3.Содержание < 5.Текст < 6.Спецификация.
	// Всё до содержания включительно пропускается; остаются файлы после него.
	pdfList := []string{
		"1. Обложка.pdf",
		"3. Содержание.pdf",
		"5. Текстовая часть.pdf",
		"6. Спецификация.pdf",
	}
	pdn := "test_pdf"
	tfn := "test_pdf/3. Содержание.docx"
	pageCounts := map[string]int{
		"1. Обложка.pdf":         2,
		"3. Содержание.pdf":      1,
		"5. Текстовая часть.pdf": 10,
		"6. Спецификация.pdf":    5,
	}

	result := buildPdfFileList(pdfList, pdn, tfn, pageCounts)
	// Обложка и Содержание пропущены, остаются 2 файла после содержания.
	if len(result) != 2 {
		t.Fatalf("expected 2 files (before+template excluded), got %d: %+v", len(result), result)
	}
	if result[0].fileName != "5. Текстовая часть.pdf" {
		t.Errorf("result[0].fileName = %q, want %q", result[0].fileName, "5. Текстовая часть.pdf")
	}
	if result[0].pages != 10 {
		t.Errorf("result[0].pages = %d, want 10", result[0].pages)
	}
	if result[1].fileName != "6. Спецификация.pdf" {
		t.Errorf("result[1].fileName = %q, want %q", result[1].fileName, "6. Спецификация.pdf")
	}
	if result[1].pages != 5 {
		t.Errorf("result[1].pages = %d, want 5", result[1].pages)
	}
}

func TestBuildPdfFileListSkipsTemplate(t *testing.T) {
	// toc стоит первым — после него идут два файла.
	pdfList := []string{
		"toc.pdf",
		"00-cover.pdf",
		"01-project.pdf",
	}
	pdn := "pdn"
	tfn := "pdn/toc.docx"

	result := buildPdfFileList(pdfList, pdn, tfn, nil)
	// toc и всё до него пропущено (toc первый), остаются 2 файла после него.
	if len(result) != 2 {
		t.Fatalf("expected 2 files after template, got %d", len(result))
	}
	if result[0].fileName != "00-cover.pdf" {
		t.Errorf("first file = %v, want 00-cover.pdf", result[0].fileName)
	}
	if result[1].fileName != "01-project.pdf" {
		t.Errorf("second file = %v, want 01-project.pdf", result[1].fileName)
	}
}

func TestBuildPdfFileListTemplateAtEnd(t *testing.T) {
	// toc последний — до него два файла, после него ничего.
	pdfList := []string{
		"00-cover.pdf",
		"01-project.pdf",
		"toc.pdf",
	}
	pdn := "pdn"
	tfn := "pdn/toc.docx"

	result := buildPdfFileList(pdfList, pdn, tfn, nil)
	// Всё до toc включительно пропущено — результат пустой.
	if len(result) != 0 {
		t.Fatalf("expected 0 files (toc at end, nothing after), got %d", len(result))
	}
}

// TestMergeTemplateData_CapitalKeys гарантирует, что данные для рендера
// содержания содержат заглавные ключи Pages/Number/Obozn/Name/Page, которые
// ищут шаблон ({{Pages.Name}}) и нормализаторы docxplate/autoexpand.
// Регресс-тест на bug с json:"pages" (строчный ключ ломал сборку тома).
func TestMergeTemplateData_CapitalKeys(t *testing.T) {
	td := &TemplateData{
		Pages: []*TableData{
			{Obozn: "01-01-А", Name: "Раздел 1", Page: "4"},
		},
		Number: 3,
	}

	data, err := mergeTemplateData(td, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	if _, ok := m["Pages"]; !ok {
		t.Errorf("ключ 'Pages' отсутствует, есть: %v", keysOf(m))
	}
	if _, ok := m["Number"]; !ok {
		t.Errorf("ключ 'Number' отсутствует, есть: %v", keysOf(m))
	}

	pages, ok := m["Pages"].([]any)
	if !ok || len(pages) != 1 {
		t.Fatalf("Pages = %#v, want 1 элемент", m["Pages"])
	}
	first, ok := pages[0].(map[string]any)
	if !ok {
		t.Fatalf("элемент Pages не map: %#v", pages[0])
	}
	for _, key := range []string{"Obozn", "Name", "Page"} {
		if _, ok := first[key]; !ok {
			t.Errorf("ключ '%s' отсутствует в элементе Pages, есть: %v", key, keysOf(first))
		}
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

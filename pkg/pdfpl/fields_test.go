package pdfpl

import "testing"

func TestFindFieldsBasic(t *testing.T) {
	stream := "BT /F1 12 Tf (Hello {{.Name}}!) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	f := fields[0]
	// Ключ поля — без точки (regexp-группа ловит имя без ведущей точки).
	if f.key != "Name" {
		t.Errorf("f.key = %q, want Name", f.key)
	}
	if f.font != "/F1" || f.size != 12 {
		t.Errorf("f.font/size = %s %g, want /F1 12", f.font, f.size)
	}
	if f.run != 0 {
		t.Errorf("f.run = %d, want 0", f.run)
	}
	if f.placeText != "Name" {
		t.Errorf("f.placeText = %q, want Name", f.placeText)
	}
	// Границы должны указывать ровно на {{.Name}}.
	if got := stream[f.begin:f.end]; got != "{{.Name}}" {
		t.Errorf("stream[f.begin:f.end] = %q, want {{.Name}}", got)
	}
}

func TestFindFieldsWhitespaceKey(t *testing.T) {
	stream := "BT /F1 12 Tf ({{ .Key }}) Tj ET"
	runs, _ := textRuns(stream)
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].key != "Key" {
		t.Errorf("f.key = %q, want Key (пробелы обрезаются)", fields[0].key)
	}
}

func TestFindFieldsWithoutDot(t *testing.T) {
	// Плейсхолдер без точки {{Key}} тоже распознаётся.
	stream := "BT /F1 12 Tf ({{Key}}) Tj ET"
	runs, _ := textRuns(stream)
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].key != "Key" {
		t.Errorf("f.key = %q, want Key", fields[0].key)
	}
}

func TestFindFieldsRangeIgnored(t *testing.T) {
	// Всё между {{range ...}} и {{end}} — целиком игнорируется.
	stream := "BT /F1 12 Tf ({{range .Items}}{{.Name}}{{end}} {{.Title}}) Tj ET"
	runs, _ := textRuns(stream)
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].key != "Title" {
		t.Errorf("f.key = %q, want Title (поля внутри range не учитываются)", fields[0].key)
	}
}

func TestFindFieldsRangeAcrossRuns(t *testing.T) {
	// {{range}} в одном ране, {{end}} в другом — пока не закрыт, поля пропускаются.
	stream := "BT /F1 12 Tf ({{range .Items}}) Tj ({{.Name}}) Tj ({{end}}) Tj ({{.Title}}) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 4 {
		t.Fatalf("len(runs) = %d, want 4", len(runs))
	}
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].key != "Title" || fields[0].run != 3 {
		t.Errorf("f = %q run %d, want Title run 3", fields[0].key, fields[0].run)
	}
}

func TestFindFieldsTjArraySplit(t *testing.T) {
	// MS Word разносит {{.ESYear}} по отдельным литералам TJ-массива.
	// findFields должен склеить их в логический текст рана.
	stream := "BT /F4 14.04 Tf 1 0 0 1 369.91 38.904 Tm 0 g 0 G [({{.)4(ESY)-3(e)8(a)-3(r})9(})] TJ ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	fields := findFields(stream, runs)
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].key != "ESYear" {
		t.Errorf("f.key = %q, want ESYear (плейсхолдер в TJ-массиве)", fields[0].key)
	}
	if fields[0].run != 0 || fields[0].font != "/F4" {
		t.Errorf("f = %s run %d, want /F4 run 0", fields[0].font, fields[0].run)
	}
}

func TestFindFieldsNoRuns(t *testing.T) {
	if fields := findFields("", nil); len(fields) != 0 {
		t.Errorf("findFields(\"\") = %d fields, want 0", len(fields))
	}
}

package pdfpl

import (
	"testing"
)

func TestTextRunsTjFontTracking(t *testing.T) {
	stream := "BT /F1 12 Tf (a) Tj /F2 10 Tf (b) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	r0, r1 := runs[0], runs[1]

	if r0.text != "a" || r0.font != "/F1" || r0.size != 12 || r0.opType != "Tj" {
		t.Errorf("r0 = %q %s %g %q, want a /F1 12 Tj", r0.text, r0.font, r0.size, r0.opType)
	}
	if r1.text != "b" || r1.font != "/F2" || r1.size != 10 || r1.opType != "Tj" {
		t.Errorf("r1 = %q %s %g %q, want b /F2 10 Tj", r1.text, r1.font, r1.size, r1.opType)
	}
	// Границы операнда: "(a) Tj" -> после "(a) Tj".
	if got := stream[r0.start:r0.end]; got != "(a) Tj" {
		t.Errorf("r0 slice = %q, want %q", got, "(a) Tj")
	}
	if got := stream[r1.start:r1.end]; got != "(b) Tj" {
		t.Errorf("r1 slice = %q, want %q", got, "(b) Tj")
	}
	if r0.opTok >= r1.opTok {
		t.Errorf("r0.opTok = %d, r1.opTok = %d, want r0 < r1", r0.opTok, r1.opTok)
	}
}

func TestTextRunsTJOrderPreserved(t *testing.T) {
	stream := "BT /F1 12 Tf [(a) -120 (b)] TJ ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	r := runs[0]
	if r.text != "ab" {
		t.Errorf("r.text = %q, want %q (порядок строк должен сохраняться)", r.text, "ab")
	}
	if r.opType != "TJ" {
		t.Errorf("r.opType = %q, want TJ", r.opType)
	}
	if got := stream[r.start:r.end]; got != "[(a) -120 (b)] TJ" {
		t.Errorf("r slice = %q, want %q", got, "[(a) -120 (b)] TJ")
	}
}

func TestTextRunsEmptyStream(t *testing.T) {
	runs, err := textRuns("")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0", len(runs))
	}
}

func TestTextRunsOutOfText(t *testing.T) {
	// Tj вне BT/ET игнорируется.
	stream := "12 0 0 12 0 0 cm (x) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0 (Tj вне BT/ET)", len(runs))
	}
}

func TestTextRunsEscapedString(t *testing.T) {
	// \\( экранированная скобка внутри литерала.
	runs, err := textRuns("BT /F1 12 Tf (a\\(b) Tj ET")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].text != "a(b" {
		t.Errorf("r.text = %q, want %q", runs[0].text, "a(b")
	}
}

func TestTextRunsLoneAngleBracket(t *testing.T) {
	// Acrobat при оптимизации нарезает контент на фрагменты «по байтам»,
	// разрывая словари: фрагмент может начинаться с одиночного '>' (хвост
	// словаря <<...>>). До исправления lexWord не продвигал курсор на
	// разделителе и токенизатор зацикливался навсегда.
	runs, err := textRuns("BT 35 >>BDC /F1 12 Tf (a) Tj ET")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].text != "a" {
		t.Errorf("r.text = %q, want a", runs[0].text)
	}
}

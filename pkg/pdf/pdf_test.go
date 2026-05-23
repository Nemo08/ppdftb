package pdf

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	pdf "github.com/oliverpool/unipdf/v3/model"
)

func a4LandscapeBox() *pdf.PdfRectangle {
	return &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 842, Ury: 595}
}

func a4PortraitBox() *pdf.PdfRectangle {
	return &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 595, Ury: 842}
}

func TestIsA4Landscape(t *testing.T) {
	rot90 := int64(90)
	rot270 := int64(270)
	rot180 := int64(180)

	tests := []struct {
		name string
		page *pdf.PdfPage
		want bool
	}{
		{
			name: "nil MediaBox",
			page: &pdf.PdfPage{},
			want: false,
		},
		{
			name: "explicit landscape",
			page: &pdf.PdfPage{MediaBox: a4LandscapeBox()},
			want: true,
		},
		{
			name: "portrait without rotation",
			page: &pdf.PdfPage{MediaBox: a4PortraitBox()},
			want: false,
		},
		{
			name: "portrait with Rotate=90",
			page: &pdf.PdfPage{MediaBox: a4PortraitBox(), Rotate: &rot90},
			want: true,
		},
		{
			name: "portrait with Rotate=270",
			page: &pdf.PdfPage{MediaBox: a4PortraitBox(), Rotate: &rot270},
			want: true,
		},
		{
			name: "portrait with Rotate=180",
			page: &pdf.PdfPage{MediaBox: a4PortraitBox(), Rotate: &rot180},
			want: false,
		},
		{
			name: "small box ignored",
			page: &pdf.PdfPage{MediaBox: &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 100, Ury: 100}},
			want: false,
		},
		{
			name: "landscape with tolerance",
			page: &pdf.PdfPage{MediaBox: &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 841, Ury: 594}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isA4Landscape(tt.page)
			if got != tt.want {
				t.Errorf("isA4Landscape() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsA4LandscapeEdgeCases(t *testing.T) {
	// Just outside tolerance
	big := &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 848, Ury: 601}
	if isA4Landscape(&pdf.PdfPage{MediaBox: big}) {
		t.Error("expected false for box outside tolerance")
	}
}

func TestMergeEmptyFolder(t *testing.T) {
	dir := t.TempDir()
	err := Merge(context.Background(), dir, filepath.Join(dir, "out.pdf"))
	if err != nil {
		t.Errorf("Merge empty folder: %v", err)
	}
	// output file should not be created for empty input
	if _, err := os.Stat(filepath.Join(dir, "out.pdf")); err == nil {
		t.Error("output file should not exist for empty input")
	}
}

func TestMergeNoPDFs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a pdf"), 0o644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("not a pdf"), 0o644)

	err := Merge(context.Background(), dir, filepath.Join(dir, "out.pdf"))
	if err != nil {
		t.Errorf("Merge with no PDFs: %v", err)
	}
}

func TestMergeNonexistentFolder(t *testing.T) {
	err := Merge(context.Background(), filepath.Join(os.TempDir(), "nonexistent_12345"), filepath.Join(os.TempDir(), "out.pdf"))
	if err == nil {
		t.Error("expected error for nonexistent folder")
	}
}

func TestPaginationConstants(t *testing.T) {
	if math.Abs(a4LandscapeW-842) > 0.001 {
		t.Errorf("a4LandscapeW = %f, want 842", a4LandscapeW)
	}
	if math.Abs(a4LandscapeH-595) > 0.001 {
		t.Errorf("a4LandscapeH = %f, want 595", a4LandscapeH)
	}
	if a4Tol != 5.0 {
		t.Errorf("a4Tol = %f, want 5.0", a4Tol)
	}
}

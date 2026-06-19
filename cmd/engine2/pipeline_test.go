//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nemo08/ppdftb/pkg/pdf"
)

func TestCopyStaticFiles_MarkerNamesWithDots(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	files := map[string][]byte{
		"10. ПРИЛОЖЕНИЯ":           {},
		"60. ГРАФИЧЕСКАЯ ЧАСТЬ":    {},
		"4. Титульный лист":        {},
		"12. Ready.pdf":            {0x25, 0x50, 0x44, 0x46},
		"3. Содержание.docx":       {0x00},
		"5. Text part.docx":        {0x00},
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(src, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	copyStaticFiles(src, dst)

	wantCopied := []string{
		"10. ПРИЛОЖЕНИЯ",
		"60. ГРАФИЧЕСКАЯ ЧАСТЬ",
		"4. Титульный лист",
		"12. Ready.pdf",
	}
	for _, name := range wantCopied {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("expected copied file %q: %v", name, err)
		}
	}

	wantSkipped := []string{"3. Содержание.docx", "5. Text part.docx"}
	for _, name := range wantSkipped {
		if _, err := os.Stat(filepath.Join(dst, name)); !os.IsNotExist(err) {
			t.Errorf("docx should not be copied: %q", name)
		}
	}

	entries, err := pdf.CollectEntries(dst, true)
	if err != nil {
		t.Fatal(err)
	}
	var dividers int
	for _, e := range entries {
		if e.Kind == pdf.KindDivider {
			dividers++
		}
	}
	if dividers < 2 {
		t.Errorf("expected appendix dividers in dst, got %d", dividers)
	}
}

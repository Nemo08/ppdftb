//go:build windows

package convert

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTplFuncs(t *testing.T) {
	tfm := tplFuncs()

	if tfm == nil {
		t.Fatal("tplFuncs() returned nil")
	}

	t.Run("add", func(t *testing.T) {
		fn, ok := tfm["add"].(func(int, int) int)
		if !ok {
			t.Fatal("add is not func(int,int) int")
		}
		if got := fn(2, 3); got != 5 {
			t.Errorf("add(2,3) = %d, want 5", got)
		}
	})

	t.Run("year returns non-empty", func(t *testing.T) {
		fn, ok := tfm["year"].(func() string)
		if !ok {
			t.Fatal("year is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("year() returned empty string")
		}
	})

	t.Run("nowdate returns date format", func(t *testing.T) {
		fn, ok := tfm["nowdate"].(func() string)
		if !ok {
			t.Fatal("nowdate is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("nowdate() returned empty string")
		}
	})

	t.Run("datetime returns datetime format", func(t *testing.T) {
		fn, ok := tfm["datetime"].(func() string)
		if !ok {
			t.Fatal("datetime is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("datetime() returned empty string")
		}
	})
}

func TestFilecopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	content := "hello world"

	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := filecopy(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(content)) {
		t.Errorf("filecopy() = %d bytes, want %d", n, len(content))
	}

	dstData, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(dstData) != content {
		t.Errorf("filecopy() content = %q, want %q", string(dstData), content)
	}
}

func TestFilecopyNonexistent(t *testing.T) {
	dir := t.TempDir()
	_, err := filecopy(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "out.txt"))
	if err == nil {
		t.Error("filecopy() expected error for nonexistent source")
	}
}

func TestCollectWordFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "b.doc"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "c.rtf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "d.pdf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "~$temp.docx"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "e.docx"), []byte{}, 0o644)

	t.Run("collect from dir", func(t *testing.T) {
		files, err := CollectWordFiles([]string{dir})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("CollectWordFiles() = %d files, want 3 (a.docx, b.doc, c.rtf — sub не рекурсивный)", len(files))
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		_, err := CollectWordFiles([]string{filepath.Join(dir, "nope")})
		if err == nil {
			t.Error("CollectWordFiles() expected error for non-existent source")
		}
	})
}

type mockWordPool struct {
	calls []string
	mu    sync.Mutex
}

func (m *mockWordPool) WordToPdf(_ context.Context, from, to string) error {
	m.mu.Lock()
	m.calls = append(m.calls, from)
	m.mu.Unlock()
	return os.WriteFile(to, []byte("mock pdf"), 0o644)
}

func (m *mockWordPool) Close() {}

func writeZipEntry(zw *zip.Writer, name, content string) {
	w, err := zw.Create(name)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		panic(err)
	}
}

func createMinimalDocx(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	writeZipEntry(zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)
	writeZipEntry(zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	writeZipEntry(zw, "word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>{{.Name}}</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`)
	writeZipEntry(zw, "word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`)
	return zw.Close()
}

func TestTplToPdfWithPool(t *testing.T) {
	dir := t.TempDir()
	tplDir := filepath.Join(dir, "tpl")
	docxOut := filepath.Join(dir, "docx")
	pdfOut := filepath.Join(dir, "pdf")
	for _, d := range []string{tplDir, docxOut, pdfOut} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()

	t.Run("empty input", func(t *testing.T) {
		mock := &mockWordPool{}
		if err := TplToPdfWithPool(ctx, mock, nil, docxOut, pdfOut, nil, ""); err != nil {
			t.Errorf("TplToPdfWithPool() = %v, want nil", err)
		}
	})

	t.Run("non-docx file copied", func(t *testing.T) {
		mock := &mockWordPool{}
		src := filepath.Join(tplDir, "readme.txt")
		if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := TplToPdfWithPool(ctx, mock, []string{src}, docxOut, pdfOut, nil, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(docxOut, "readme.txt")); os.IsNotExist(err) {
			t.Error("non-docx file should be copied to docxOut")
		}
		if len(mock.calls) != 0 {
			t.Errorf("WordToPdf called %d times, want 0 for non-docx", len(mock.calls))
		}
	})

	t.Run("valid template produces docx and pdf", func(t *testing.T) {
		mock := &mockWordPool{}
		tpl := filepath.Join(tplDir, "test.docx")
		if err := createMinimalDocx(tpl); err != nil {
			t.Fatal(err)
		}

		data := []byte(`{"Name":"TestValue"}`)
		if err := TplToPdfWithPool(ctx, mock, []string{tpl}, docxOut, pdfOut, data, ""); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(docxOut, "test.docx")); os.IsNotExist(err) {
			t.Error("docx output not found")
		}
		if _, err := os.Stat(filepath.Join(pdfOut, "test.pdf")); os.IsNotExist(err) {
			t.Error("pdf output not found")
		}
		if len(mock.calls) != 1 {
			t.Errorf("WordToPdf called %d times, want 1", len(mock.calls))
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		mock := &mockWordPool{}
		names := []string{"a.docx", "b.docx", "c.docx"}
		for _, n := range names {
			if err := createMinimalDocx(filepath.Join(tplDir, n)); err != nil {
				t.Fatal(err)
			}
		}

		var inputs []string
		for _, n := range names {
			inputs = append(inputs, filepath.Join(tplDir, n))
		}

		if err := TplToPdfWithPool(ctx, mock, inputs, docxOut, pdfOut, []byte(`{"Name":"X"}`), ""); err != nil {
			t.Fatal(err)
		}

		for _, n := range names {
			base := strings.TrimSuffix(n, ".docx")
			if _, err := os.Stat(filepath.Join(docxOut, base+".docx")); os.IsNotExist(err) {
				t.Errorf("docx output %s not found", n)
			}
			if _, err := os.Stat(filepath.Join(pdfOut, base+".pdf")); os.IsNotExist(err) {
				t.Errorf("pdf output %s.pdf not found", base)
			}
		}
		if len(mock.calls) != 3 {
			t.Errorf("WordToPdf called %d times, want 3", len(mock.calls))
		}
	})
}

// docxconv конвертирует .docx файлы в PDF через docx2pdf-go.
package docxconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"log/slog"

	docx2pdf "github.com/bobyeoh/docx2pdf-go"
)

// Convert конвертирует один .docx файл в PDF.
func Convert(ctx context.Context, src, dst string) error {
	if err := docx2pdf.Convert(src, dst, docx2pdf.Options{}); err != nil {
		return err
	}
	slog.Default().DebugContext(ctx, "конвертирован", slog.String("src", src), slog.String("dst", dst))
	return nil
}

// ConvertDir конвертирует все .docx файлы из папки srcDir в PDF в папку dstDir.
func ConvertDir(ctx context.Context, srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".docx" {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			src := filepath.Join(srcDir, name)
			dst := filepath.Join(dstDir, strings.TrimSuffix(name, ext)+".pdf")
			if err := Convert(ctx, src, dst); err != nil {
				slog.Default().ErrorContext(ctx, "docx2pdf",
					slog.String("file", src),
					slog.String("err", err.Error()),
				)
			}
		}(entry.Name())
	}
	wg.Wait()
	return nil
}

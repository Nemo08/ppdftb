package convert

import "context"

// WordConverter — интерфейс конвертации Word → PDF.
type WordConverter interface {
	WordToPdf(ctx context.Context, fromWordFile, toPdfFile string) error
	Close()
}

// CadConverter — интерфейс конвертации AutoCAD → PDF.
type CadConverter interface {
	AcadToPdf(ctx context.Context, fromFile, toDir string) error
	Close()
}

// PdfMerger — интерфейс слияния PDF из папки в один файл.
type PdfMerger interface {
	Merge(ctx context.Context, srcDir, dstFile string) error
}

// ConvCache — кэш для отслеживания изменений файлов.
// Используется как альтернатива прямому вызову cache.FilesToConvert.
type ConvCache interface {
	FilesToConvert(inDirs []string, xmlFiles []string, outDir string, withHash bool) ([]string, error)
	CommitCache(dirs []string, exts map[string]bool, withHash bool) error
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"log/slog"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/dataconv"
	"github.com/Nemo08/ppdftb/pkg/fileutil"
	"github.com/Nemo08/ppdftb/pkg/jobutil"
	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/toc"
	"github.com/Nemo08/ppdftb/pkg/wordpool"
)

type Config struct {
	RootDir     string
	TplDir      string
	TocTemplate string // файл шаблона содержания (-tf)
	OutFile     string
	PDFDir      string
	DocsDir     string
	PicsDir     string
	WordPool    int
	TocPageFrom int // номер страницы оглавления в итоговом PDF (-tn)
	PageFrom    int // с какой страницы начинать нумерацию (-pf)
	NumberFrom  int // начальный номер (-nf)
	XMLDepth    int // на сколько папок выше смотреть (-u)
}

func Run(ctx context.Context, cfg *Config) error {
	rootDir, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return fmt.Errorf("root dir: %w", err)
	}

	tplDir := resolveDir(rootDir, cfg.TplDir, "Шаблон тома")
	pdfDir := resolveDir(rootDir, cfg.PDFDir, "PDF")
	docsDir := resolveDir(rootDir, cfg.DocsDir, "Документы тома")
	picsDir := resolveDir(rootDir, cfg.PicsDir, "pics")
	tempPDF := filepath.Join(rootDir, "temp.pdf")

	outFile, err := resolveOutFile(rootDir, cfg.OutFile)
	if err != nil {
		return fmt.Errorf("выходной файл: %w", err)
	}

	// Разрешаем путь к шаблону содержания (-tf).
	tocTemplate := cfg.TocTemplate
	if tocTemplate == "" {
		return fmt.Errorf("обязательный флаг -tf (файл шаблона содержания)")
	}
	if !filepath.IsAbs(tocTemplate) {
		tocTemplate = filepath.Join(rootDir, tocTemplate)
	}
	if _, err := os.Stat(tocTemplate); err != nil {
		return fmt.Errorf("шаблон содержания не найден: %w", err)
	}
	contentName := filepath.Base(tocTemplate)

	slog.Info("engine2 pipeline",
		slog.String("tpl", tplDir),
		slog.String("tocTemplate", tocTemplate),
		slog.String("pdf", pdfDir),
		slog.String("docs", docsDir),
		slog.String("pics", picsDir),
		slog.String("out", outFile),
		slog.Int("tocPage", cfg.TocPageFrom),
		slog.Int("pageFrom", cfg.PageFrom),
		slog.Int("numberFrom", cfg.NumberFrom),
		slog.Int("xmlDepth", cfg.XMLDepth),
	)

	if err := ensureDirs(pdfDir, docsDir); err != nil {
		return err
	}

	pool := wordpool.NewWordPool(cfg.WordPool)
	defer pool.Close()

	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := pool.WaitReady(readyCtx); err != nil {
		return fmt.Errorf("Word pool: %w", err)
	}

	cleanDir(pdfDir)
	cleanDir(docsDir)
	_ = os.Remove(tempPDF)
	_ = os.Remove(outFile)

	// Готовые PDF и маркеры-разделители (файлы без расширения) — как copy "%tpl%\*" в .cmd.
	copyStaticFiles(tplDir, pdfDir)

	if err := runWconvPass(ctx, pool, tplDir, pdfDir, docsDir, rootDir, picsDir, cfg.XMLDepth); err != nil {
		return fmt.Errorf("wconv pass 1: %w", err)
	}

	// Данные штампа (XML) для содержания — тот же источник и уровень поиска,
	// что и в основном проходе wconv (DxF=rootDir, DxL=1). Считаем один раз:
	// содержание при обоих проходах toc→wconv шаблонизируется одними и теми же
	// общими данными, меняются только номера страниц (их подставляет toc.Make).
	tocData, err := collectTocData(rootDir)
	if err != nil {
		return fmt.Errorf("XML для содержания: %w", err)
	}

	// Два прохода toc→wconv для сходимости номеров страниц (как в engine.cmd).
	for i := 0; i < 2; i++ {
		pageCounts := collectPageCounts(pdfDir)
		slog.Debug("toc pass", slog.Int("pass", i+1), slog.Int("pdfs", len(pageCounts)))

		if err := toc.Make(ctx,
			tocTemplate,
			pdfDir,
			docsDir,
			cfg.TocPageFrom,
			toc.WithAppendix(),
			toc.WithPageCounts(pageCounts),
			toc.WithTemplateData(tocData),
		); err != nil {
			return fmt.Errorf("toc pass %d: %w", i+1, err)
		}

		// toc.Make заполняет только номера страниц (TemplateData: Pages/Number),
		// оставляя прочие плейсхолдеры нетронутыми (WithIgnoreMissingKey).
		// Здесь донабиваем их общими данными штампа — итоговый docx содержит
		// и актуальную нумерацию, и общие поля тома.
		if err := conv.TplToPdfWithPool(ctx, pool,
			[]string{filepath.Join(docsDir, contentName)},
			docsDir, pdfDir,
			tocData, picsDir,
		); err != nil {
			return fmt.Errorf("wconv toc->pdf pass %d: %w", i+1, err)
		}
	}

	if err := pdf.Merge(ctx, pdfDir, tempPDF, pdf.WithAppendix()); err != nil {
		return fmt.Errorf("mpdf: %w", err)
	}
	if err := pdf.MakePagination(ctx, tempPDF, outFile, cfg.PageFrom, cfg.NumberFrom, pdf.WithPaginationAppendix()); err != nil {
		return fmt.Errorf("pnpdf: %w", err)
	}
	_ = os.Remove(tempPDF)

	slog.Info("done", slog.String("output", outFile))
	return nil
}

func resolveOutFile(rootDir, outFile string) (string, error) {
	if outFile != "" {
		if !filepath.IsAbs(outFile) {
			outFile = filepath.Join(rootDir, outFile)
		}
		return filepath.Abs(outFile)
	}
	name, err := resolveOutputName(rootDir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(rootDir, name))
}

func resolveOutputName(rootDir string) (string, error) {
	data, _, err := fileutil.FindXMLFiles(rootDir, 0)
	if err != nil {
		return "", fmt.Errorf("find xml: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("xml not found in %s", rootDir)
	}
	merged, err := dataconv.DataMerge(data)
	if err != nil {
		return "", fmt.Errorf("merge xml: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		return "", fmt.Errorf("parse merged data: %w", err)
	}
	esNum, _ := m["ESNumber"].(string)
	esType, _ := m["ESType"].(string)
	if esNum == "" || esType == "" {
		return "", fmt.Errorf("ESNumber или ESType не найдены в XML")
	}
	esNum = strings.ReplaceAll(esNum, "/", "-")
	return fmt.Sprintf("%s-%s.pdf", esNum, esType), nil
}

func runWconvPass(ctx context.Context, pool *wordpool.WordPool, tplDir, pdfDir, docsDir, rootDir, picsDir string, xmlDepth int) error {
	p := &conv.WconvPipeline{
		Src:      tplDir,
		Out:      pdfDir,
		Outd:     docsDir,
		DxF:      rootDir,
		DxL:      xmlDepth,
		PicsDir:  picsDir,
		UseCache: true,
		Cache:    cache.ConvCache{},
	}
	return conv.RunWconvWithPool(ctx, pool, p)
}

// collectTocData собирает и мёрджит XML-данные из rootDir тем же способом
// (уровень поиска 1), что и WconvPipeline.DxL в runWconvPass, — чтобы шаблон
// содержания получал те же общие поля штампа, что и остальные документы тома.
func collectTocData(rootDir string) ([]byte, error) {
	data, _, err := fileutil.FindXMLFiles(rootDir, 1)
	if err != nil {
		return nil, fmt.Errorf("поиск XML: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	merged, err := dataconv.DataMerge(data)
	if err != nil {
		return nil, fmt.Errorf("слияние XML: %w", err)
	}
	return merged, nil
}

func collectPageCounts(pdfDir string) map[string]int {
	files, err := fileutil.CollectFiles([]string{pdfDir}, []string{".pdf"})
	if err != nil {
		return nil
	}
	counts := make(map[string]int, len(files))
	var mu sync.Mutex
	jobutil.Parallel(8, files, func(f string) {
		n, err := pdf.PageCount(f)
		if err != nil || n <= 0 {
			return
		}
		mu.Lock()
		counts[filepath.Base(f)] = n
		mu.Unlock()
	})
	return counts
}

func resolveDir(rootDir, dir, defaultName string) string {
	if dir == "" {
		dir = defaultName
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(rootDir, dir)
	}
	return dir
}

func ensureDirs(dirs ...string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func cleanDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// copyStaticFiles копирует из шаблона в PDF-папку готовые PDF и маркеры-разделители
// (имена с точками, но без известного расширения — «10. ПРИЛОЖЕНИЯ» и т.п.).
// DOCX/DOC пропускаются: их конвертирует wconv.
func copyStaticFiles(src, dst string) {
	_ = os.MkdirAll(dst, 0755)
	entries, err := os.ReadDir(src)
	if err != nil {
		slog.Warn("copy static: read dir", slog.String("err", err.Error()))
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := pdf.ExtOf(e.Name())
		if ext != ".pdf" && ext != "" {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			slog.Warn("copy static: read", slog.String("file", srcPath), slog.String("err", err.Error()))
			continue
		}
		dstPath := filepath.Join(dst, e.Name())
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			slog.Warn("copy static: write", slog.String("file", dstPath), slog.String("err", err.Error()))
		}
	}
}

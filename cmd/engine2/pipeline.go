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

// volPaths — разрешённые абсолютные пути тома, вычисленные один раз в Run.
type volPaths struct {
	rootDir     string
	tplDir      string
	pdfDir      string
	docsDir     string
	picsDir     string
	tempPDF     string
	outFile     string
	tocTemplate string
	contentName string
}

func Run(ctx context.Context, cfg *Config) error {
	vol, err := resolveVolumePaths(cfg)
	if err != nil {
		return err
	}

	logVolume(vol, cfg)

	if err := ensureDirs(vol.pdfDir, vol.docsDir); err != nil {
		return err
	}

	pool := wordpool.NewWordPool(cfg.WordPool)
	defer pool.Close()

	if err := waitWordReady(ctx, pool); err != nil {
		return err
	}

	cleanVolume(vol)

	// Готовые PDF и маркеры-разделители (файлы без расширения) — как copy "%tpl%\*" в .cmd.
	copyStaticFiles(vol.tplDir, vol.pdfDir)

	if err := runWconvPass(ctx, pool, vol, cfg.XMLDepth); err != nil {
		return fmt.Errorf("wconv pass 1: %w", err)
	}

	// Данные штампа (XML) для содержания — тот же источник и уровень поиска,
	// что и в основном проходе wconv (DxF=rootDir, DxL=1). Считаем один раз:
	// содержание при обоих проходах toc→wconv шаблонизируется одними и теми же
	// общими данными, меняются только номера страниц (их подставляет toc.Make).
	tocData, err := collectTocData(vol.rootDir)
	if err != nil {
		return fmt.Errorf("XML для содержания: %w", err)
	}

	if err := runTocPasses(ctx, pool, vol, cfg.TocPageFrom, tocData); err != nil {
		return err
	}

	return finalizeVolume(ctx, vol, cfg.PageFrom, cfg.NumberFrom)
}

// resolveVolumePaths разрешает корень тома, рабочие папки, выходной файл
// и шаблон содержания (-tf) в абсолютные пути.
func resolveVolumePaths(cfg *Config) (*volPaths, error) {
	rootDir, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("root dir: %w", err)
	}

	tplDir := resolveDir(rootDir, cfg.TplDir, "Шаблон тома")
	pdfDir := resolveDir(rootDir, cfg.PDFDir, "PDF")
	docsDir := resolveDir(rootDir, cfg.DocsDir, "Документы тома")
	picsDir := cfg.PicsDir
	if picsDir != "" {
		if !filepath.IsAbs(picsDir) {
			picsDir = filepath.Join(rootDir, picsDir)
		}
		if st, err := os.Stat(picsDir); err != nil || !st.IsDir() {
			slog.Warn("папка с картинками -pp не найдена, подстановка отключена", slog.String("dir", picsDir))
			picsDir = ""
		}
	}
	tempPDF := filepath.Join(rootDir, "temp.pdf")

	outFile, err := resolveOutFile(rootDir, cfg.OutFile)
	if err != nil {
		return nil, fmt.Errorf("выходной файл: %w", err)
	}

	// Разрешаем путь к шаблону содержания (-tf).
	tocTemplate := cfg.TocTemplate
	if tocTemplate == "" {
		return nil, fmt.Errorf("обязательный флаг -tf (файл шаблона содержания)")
	}
	if !filepath.IsAbs(tocTemplate) {
		tocTemplate = filepath.Join(rootDir, tocTemplate)
	}
	if _, err := os.Stat(tocTemplate); err != nil {
		return nil, fmt.Errorf("шаблон содержания не найден: %w", err)
	}

	return &volPaths{
		rootDir:     rootDir,
		tplDir:      tplDir,
		pdfDir:      pdfDir,
		docsDir:     docsDir,
		picsDir:     picsDir,
		tempPDF:     tempPDF,
		outFile:     outFile,
		tocTemplate: tocTemplate,
		contentName: filepath.Base(tocTemplate),
	}, nil
}

func logVolume(vol *volPaths, cfg *Config) {
	slog.Info("engine2 pipeline",
		slog.String("tpl", vol.tplDir),
		slog.String("tocTemplate", vol.tocTemplate),
		slog.String("pdf", vol.pdfDir),
		slog.String("docs", vol.docsDir),
		slog.String("pics", vol.picsDir),
		slog.String("out", vol.outFile),
		slog.Int("tocPage", cfg.TocPageFrom),
		slog.Int("pageFrom", cfg.PageFrom),
		slog.Int("numberFrom", cfg.NumberFrom),
		slog.Int("xmlDepth", cfg.XMLDepth),
	)
}

func waitWordReady(ctx context.Context, pool *wordpool.WordPool) error {
	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := pool.WaitReady(readyCtx); err != nil {
		return fmt.Errorf("Word pool: %w", err)
	}
	return nil
}

func cleanVolume(vol *volPaths) {
	cleanDir(vol.pdfDir)
	cleanDir(vol.docsDir)
	_ = os.Remove(vol.tempPDF)
	_ = os.Remove(vol.outFile)
}

// runWconvPass конвертирует DOCX-шаблоны тома в PDF через пул Word.
func runWconvPass(ctx context.Context, pool *wordpool.WordPool, vol *volPaths, xmlDepth int) error {
	p := &conv.WconvPipeline{
		Src:      vol.tplDir,
		Out:      vol.pdfDir,
		Outd:     vol.docsDir,
		DxF:      vol.rootDir,
		DxL:      xmlDepth,
		PicsDir:  vol.picsDir,
		UseCache: true,
		Cache:    cache.ConvCache{},
	}
	return conv.RunWconvWithPool(ctx, pool, p)
}

// runTocPasses выполняет два прохода toc→wconv для сходимости номеров
// страниц (как в engine.cmd).
func runTocPasses(ctx context.Context, pool *wordpool.WordPool, vol *volPaths, tocPageFrom int, tocData []byte) error {
	for i := 0; i < 2; i++ {
		pageCounts := collectPageCounts(vol.pdfDir)
		slog.Debug("toc pass", slog.Int("pass", i+1), slog.Int("pdfs", len(pageCounts)))

		if err := toc.Make(ctx,
			vol.tocTemplate,
			vol.pdfDir,
			vol.docsDir,
			tocPageFrom,
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
			[]string{filepath.Join(vol.docsDir, vol.contentName)},
			vol.docsDir, vol.pdfDir,
			tocData, vol.picsDir,
		); err != nil {
			return fmt.Errorf("wconv toc->pdf pass %d: %w", i+1, err)
		}
	}
	return nil
}

// finalizeVolume собирает итоговый PDF тома: слияние и нумерация страниц.
func finalizeVolume(ctx context.Context, vol *volPaths, pageFrom, numberFrom int) error {
	if err := pdf.Merge(ctx, vol.pdfDir, vol.tempPDF, pdf.WithAppendix()); err != nil {
		return fmt.Errorf("mpdf: %w", err)
	}
	if err := pdf.MakePagination(ctx, vol.tempPDF, vol.outFile, pageFrom, numberFrom, pdf.WithPaginationAppendix()); err != nil {
		return fmt.Errorf("pnpdf: %w", err)
	}
	_ = os.Remove(vol.tempPDF)

	slog.Info("done", slog.String("output", vol.outFile))
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

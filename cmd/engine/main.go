//go:build windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"log/slog"

	acadpool "github.com/Nemo08/ppdftb/pkg/acadpool"
	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/jobutil"
	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
	"github.com/Nemo08/ppdftb/pkg/toc"
	wordpool "github.com/Nemo08/ppdftb/pkg/wordpool"
)

// compile-time проверки.
var _ conv.WordConverter = (*wordpool.WordPool)(nil)
var _ conv.CadConverter = (*acadpool.AcadPool)(nil)
var _ conv.ConvCache = cache.ConvCache{}

const defaultPort = 17321

type stringSlice []string

func (s *stringSlice) String() string { return "" }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type jobRequest struct {
	Tool string   `json:"tool"`
	Args []string `json:"args"`
}

type jobResponse struct {
	Error string `json:"error,omitempty"`
}

func main() {
	var Level, CPUProfile string
	var Version, Serve, Shutdown bool
	var Port, WordPoolSize, AcadPoolSize int

	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&Serve, "serve", false, "режим сервера")
	flag.BoolVar(&Shutdown, "shutdown", false, "остановить сервер")
	flag.IntVar(&Port, "port", defaultPort, "порт TCP-сервера")
	flag.IntVar(&WordPoolSize, "wordpool", 4, "размер пула Word (кол-во параллельных конвертаций)")
	flag.IntVar(&AcadPoolSize, "acadpool", 1, "размер пула AutoCAD (кол-во параллельных конвертаций)")
	flag.StringVar(&CPUProfile, "cpuprofile", "", "писать CPU профиль в файл")

	flag.Parse()

	slogutil.Setup(Level)

	if Version {
		fmt.Println("ppdftb engine")
		return
	}

	if CPUProfile != "" {
		f, err := os.Create(CPUProfile)
		if err != nil {
			slog.Error("cpuprofile", slog.String("err", err.Error()))
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
		}()
	}

	if Serve {
		runServer(Port, WordPoolSize, AcadPoolSize)
		return
	}

	if Shutdown {
		if err := sendRequest(Port, "shutdown", nil); err != nil {
			slog.Error("остановка сервера", slog.String("err", err.Error()))
			os.Exit(1)
		}
		fmt.Println("Сервер остановлен")
		return
	}

	// Client mode: первый позиционный аргумент — имя утилиты.
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Использование: engine [-serve | -shutdown | <утилита> <аргументы...>]")
		os.Exit(1)
	}

	if err := sendRequest(Port, args[0], args[1:]); err != nil {
		slog.Error("ошибка", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func sendRequest(port int, tool string, args []string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(jobRequest{Tool: tool, Args: args}); err != nil {
		return err
	}

	var resp jobResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func runServer(port, wordPoolSize, acadPoolSize int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Debug("engine запущен", slog.String("addr", addr),
		slog.Int("wordpool", wordPoolSize), slog.Int("acadpool", acadPoolSize))

	wordPool := wordpool.NewWordPool(wordPoolSize)
	defer wordPool.Close()

	acadPool := acadpool.NewAcadPool(acadPoolSize)
	defer acadPool.Close()

	shutdownCh := make(chan struct{})
	var shutdownOnce sync.Once
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			slog.Debug("остановка по сигналу")
		case <-shutdownCh:
			slog.Debug("остановка по команде")
		}
		ln.Close()
	}()

	pageCache := &sync.Map{} // absPath → int (количество страниц)

	var inflight sync.WaitGroup
	handlers := map[string]HandlerFunc{
		"wconv": func(args []string) error {
			inflight.Add(1)
			defer inflight.Done()
			return runWconv(wordPool, args, pageCache)
		},
		"aconv": func(args []string) error { inflight.Add(1); defer inflight.Done(); return runAconv(acadPool, args) },
		"toc":   func(args []string) error { inflight.Add(1); defer inflight.Done(); return runToc(args, pageCache) },
		"mpdf":  func(args []string) error { inflight.Add(1); defer inflight.Done(); return runMpdf(args) },
		"pnpdf": func(args []string) error { inflight.Add(1); defer inflight.Done(); return runPnpdf(args) },
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		go handleConn(conn, handlers, shutdownCh, &shutdownOnce)
	}

	slog.Debug("ожидание завершения заданий...")
	inflight.Wait()
}

// HandlerFunc — обработчик запроса к engine.
type HandlerFunc func(args []string) error

func handleConn(conn net.Conn, handlers map[string]HandlerFunc, shutdownCh chan struct{}, shutdownOnce *sync.Once) {
	defer conn.Close()

	var req jobRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(jobResponse{Error: err.Error()})
		return
	}

	slog.Debug("задание", slog.String("tool", req.Tool), slog.Any("args", req.Args))

	var runErr error
	if h, ok := handlers[req.Tool]; ok {
		runErr = h(req.Args)
	} else if req.Tool == "shutdown" {
		shutdownOnce.Do(func() { close(shutdownCh) })
	} else {
		runErr = fmt.Errorf("неизвестная утилита: %s", req.Tool)
	}

	resp := jobResponse{}
	if runErr != nil {
		resp.Error = runErr.Error()
		slog.Error("ошибка выполнения", slog.String("tool", req.Tool), slog.String("err", runErr.Error()))
	}
	json.NewEncoder(conn).Encode(resp)
}

// pageEntry — значение в pageCache: количество страниц + метаданные для проверки актуальности.
type pageEntry struct {
	Pages   int
	ModTime time.Time
	Size    int64
}

// collectPageCounts сканирует папку с PDF, читает количество страниц и сохраняет в кэш.
// Пропускает PDF, которые уже есть в кэше и не изменились (modTime+size).
// Чтение страниц выполняется параллельно (до 8 горутин одновременно).
func collectPageCounts(pdfDir string, cache *sync.Map) {
	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		return
	}
	var jobs []string
	for _, e := range entries {
		if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".pdf" {
			continue
		}
		fullPath := filepath.Join(pdfDir, e.Name())
		if isCachedUpToDate(fullPath, cache) {
			continue
		}
		jobs = append(jobs, fullPath)
	}
	jobutil.Parallel(8, jobs, func(fp string) {
		pages := readPageCount(fp)
		if pages > 0 {
			cachePage(fp, pages, cache)
		}
	})
}

func isCachedUpToDate(fullPath string, cache *sync.Map) bool {
	v, ok := cache.Load(fullPath)
	if !ok {
		return false
	}
	entry := v.(pageEntry)
	stat, err := os.Stat(fullPath)
	if err != nil {
		return false
	}
	return stat.ModTime().Equal(entry.ModTime) && stat.Size() == entry.Size
}

func readPageCount(fullPath string) int {
	n, err := pdf.PageCount(fullPath)
	if err != nil {
		return 0
	}
	return n
}

func cachePage(fullPath string, pages int, cache *sync.Map) {
	if stat, err := os.Stat(fullPath); err == nil {
		cache.Store(fullPath, pageEntry{Pages: pages, ModTime: stat.ModTime(), Size: stat.Size()})
	} else {
		cache.Store(fullPath, pageEntry{Pages: pages})
	}
}

// runWconv — логика wconv с переданным WordPool.
func runWconv(pool *wordpool.WordPool, args []string, pageCache *sync.Map) error {
	fs := flag.NewFlagSet("wconv", flag.ContinueOnError)
	var Src, Out, Outd string
	var UseCache bool
	var DxF, PicsDir string
	var DxL int
	var Dx stringSlice

	fs.StringVar(&Src, "s", "", "")
	fs.StringVar(&Out, "o", "", "")
	fs.StringVar(&Outd, "d", "", "")
	fs.BoolVar(&UseCache, "c", false, "")
	fs.Var(&Dx, "i", "")
	fs.StringVar(&DxF, "x", "", "")
	fs.IntVar(&DxL, "u", 0, "")
	fs.StringVar(&PicsDir, "p", "", "")
	var ignoreLevel string
	fs.StringVar(&ignoreLevel, "l", "", "")
	var version bool
	fs.BoolVar(&version, "v", false, "")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if Out == "" {
		return fmt.Errorf("Должна быть указана папка для PDF (-o)")
	}

	p := &conv.WconvPipeline{
		Src:      Src,
		Out:      strings.TrimRight(Out, `/\`),
		Outd:     strings.TrimRight(Outd, `/\`),
		DxFlags:  Dx,
		DxF:      DxF,
		DxL:      DxL,
		PicsDir:  PicsDir,
		UseCache: UseCache,
		Cache:    nil,
	}
	if UseCache {
		p.Cache = cache.ConvCache{}
	}
	if err := conv.RunWconvWithPool(context.Background(), pool, p); err != nil {
		return err
	}
	// После успешной конвертации собираем количество страниц в выходных PDF.
	collectPageCounts(p.Out, pageCache)
	return nil
}

type pdfMergerAdapter struct{}

func (pdfMergerAdapter) Merge(ctx context.Context, srcDir, dstFile string) error {
	return pdf.Merge(ctx, srcDir, dstFile)
}

// runToc — оглавление напрямую (без subprocess), с использованием pageCache.
func runToc(args []string, pageCache *sync.Map) error {
	// Парсим флаги как в cmd/toc/main.go
	fs := flag.NewFlagSet("toc", flag.ContinueOnError)
	var tf, td, pd string
	var tn int
	fs.StringVar(&tf, "tf", "", "")
	fs.StringVar(&td, "td", "", "")
	fs.StringVar(&pd, "pd", "", "")
	fs.IntVar(&tn, "tn", 3, "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var src, pdfDir, out string
	page := tn

	if tf != "" {
		src = tf
		out = td
		pdfDir = pd
	} else {
		rest := fs.Args()
		if len(rest) < 3 {
			return fmt.Errorf("Обязательные аргументы: source-file output-folder pdf-folder [page]")
		}
		src = rest[0]
		out = rest[1]
		pdfDir = rest[2]
		if len(rest) > 3 {
			var err error
			page, err = strconv.Atoi(rest[3])
			if err != nil {
				return fmt.Errorf("page должен быть числом")
			}
		}
	}

	slog.Debug("runToc args", slog.String("src", src), slog.String("out", out), slog.String("pdfDir", pdfDir), slog.Int("page", page))

	if src == "" || out == "" || pdfDir == "" {
		return fmt.Errorf("Обязательные аргументы: source, output, pdf")
	}

	// Собираем pageCounts для PDF из кэша.
	counts := make(map[string]int, 10)
	absPdfDir, _ := filepath.Abs(pdfDir)
	pageCache.Range(func(k, v any) bool {
		absPath, _ := k.(string)
		entry, _ := v.(pageEntry)
		if filepath.Dir(absPath) == absPdfDir {
			counts[filepath.Base(absPath)] = entry.Pages
		}
		return true
	})

	return toc.Make(context.Background(), src, pdfDir, out, page, toc.WithPageCounts(counts))
}

// runAconv — логика aconv с переданным AcadPool.
func runAconv(pool *acadpool.AcadPool, args []string) error {
	fs := flag.NewFlagSet("aconv", flag.ContinueOnError)
	var SrcFile, SrcDir, Out string

	fs.StringVar(&SrcFile, "if", "", "")
	fs.StringVar(&SrcDir, "id", "", "")
	fs.StringVar(&Out, "od", "", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	Out = strings.TrimRight(Out, `/\`)
	if Out == "" {
		return fmt.Errorf("Должна быть указана папка для PDF (-od)")
	}

	ctx := context.Background()

	inputCadFiles, err := conv.CollectCadFiles(SrcFile, SrcDir)
	if err != nil {
		return err
	}
	if len(inputCadFiles) == 0 {
		slog.Debug("Нет DWG/DXF файлов для конвертации")
		return nil
	}

	return conv.A2pdfWithPool(ctx, pool, pdfMergerAdapter{}, inputCadFiles, Out)
}

func runMpdf(args []string) error {
	var srcDir, outFile string
	fs := flag.NewFlagSet("mpdf", flag.ContinueOnError)
	fs.StringVar(&srcDir, "d", "", "")
	fs.StringVar(&outFile, "o", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if srcDir == "" {
		return fmt.Errorf("Должна быть указана папка с PDF (-d)")
	}

	return pdf.Merge(context.Background(), srcDir, outFile)
}

func runPnpdf(args []string) error {
	var inFile, outFile string
	var pageFrom, numberFrom int
	fs := flag.NewFlagSet("pnpdf", flag.ContinueOnError)
	fs.StringVar(&inFile, "if", "", "")
	fs.StringVar(&outFile, "of", "", "")
	fs.IntVar(&pageFrom, "pf", 1, "")
	fs.IntVar(&numberFrom, "nf", 1, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if inFile == "" {
		return fmt.Errorf("Должен быть указан входной файл (-if)")
	}
	if outFile == "" {
		return fmt.Errorf("Должен быть указан выходной файл (-of)")
	}

	return pdf.MakePagination(context.Background(), inFile, outFile, pageFrom, numberFrom)
}

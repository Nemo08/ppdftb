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
	"strings"
	"syscall"
	"time"

	"log/slog"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

type stringSlice []string

func (s *stringSlice) String() string { return "" }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

var version string

const defaultPort = 17321

type jobRequest struct {
	Sources  []string `json:"sources"`
	OutDir   string   `json:"outDir"`
	Shutdown bool     `json:"shutdown"`
}

type jobResponse struct {
	Error string `json:"error,omitempty"`
}

func main() {
	var Src, Out, Outd, Level string
	var Version, UseCache, Serve, Shutdown bool
	var DxF, PicsDir string
	var DxL int
	var Dx stringSlice
	var Port int

	flag.StringVar(&Src, "s", "", "файл или папка для конвертации")
	flag.StringVar(&Out, "o", "", "папка для сконвертированных *.pdf файлов")
	flag.StringVar(&Outd, "d", "", "папка для собранных *.docx файлов")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&UseCache, "c", false, "использовать кэш (-c)")
	flag.Var(&Dx, "i", "данные для шаблона (файл .xml/.json)")
	flag.StringVar(&DxF, "x", "", "корневая папка с файлами *.xml данных для шаблона")
	flag.IntVar(&DxL, "u", 0, "на сколько папок выше смотреть")
	flag.StringVar(&PicsDir, "p", "", "папка с картинками для подстановки в шаблон")
	flag.BoolVar(&Serve, "serve", false, "режим сервера (WordPool живёт между вызовами)")
	flag.BoolVar(&Shutdown, "shutdown", false, "остановить работающий сервер")
	flag.IntVar(&Port, "port", defaultPort, "порт TCP-сервера")

	flag.Parse()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}

	if Serve {
		runServer(Port)
		return
	}

	if Shutdown {
		if err := sendShutdown(Port); err != nil {
			slog.Error("остановка сервера", slog.String("err", err.Error()))
			os.Exit(1)
		}
		fmt.Println("Сервер остановлен")
		return
	}

	// Windows: путь вида "F:\path\" — финальный слэш экранирует закрывающую кавычку в CMD.
	Out = strings.TrimRight(Out, `/\`)
	if Outd != "" {
		Outd = strings.TrimRight(Outd, `/\`)
	}

	if Out == "" {
		slog.Error("Должна быть указана папка для PDF (-o)")
		os.Exit(1)
	}

	ctx := context.Background()

	var tempDir string
	var err error

	if Outd == "" {
		tempDir, err = os.MkdirTemp(os.TempDir(), "wconv")
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		defer os.RemoveAll(tempDir)
	} else {
		tempDir = Outd
	}

	var data [][]byte
	var xmlPaths []string

	// Собираем файлы через -x (от верхних папок к нижним).
	if DxF != "" {
		xData, xPaths, err := conv.FindXMLFiles(DxF, DxL)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	// Затем добавляем явно указанные файлы через -i (перекрывают -x).
	if len(Dx) != 0 {
		iData, err := conv.GetDataContent(ctx, Dx)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, Dx...)
	}

	var mergedData []byte
	if len(data) > 0 {
		mergedData, err = conv.DataMerge(data)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка данных", slog.Any("err", err))
			os.Exit(1)
		}
	}

	var toConvertList []string
	sources := []string{Src}
	if UseCache {
		toConvertList, err = cache.FilesToConvert(sources, xmlPaths, Out, false)
		if err != nil {
			slog.Error("определить список файлов", slog.String("err", err.Error()))
			os.Exit(1)
		}
	} else {
		toConvertList, err = conv.CollectWordFiles(sources)
		if err != nil {
			slog.Error("собрать файлы", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	if len(toConvertList) == 0 {
		if UseCache {
			fmt.Println("Файлы не изменились, конвертировать нечего.")
		} else {
			fmt.Println("Нет файлов для конвертации в", Src)
		}
		os.Exit(0)
	}

	err = conv.TplToDocxJJack3(ctx, toConvertList, tempDir, mergedData, PicsDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка шаблонов", slog.Any("err", err))
		os.Exit(1)
	}

	// Пробуем отправить задание серверу. Если сервер недоступен — работаем локально.
	if !tryServerJob(Port, []string{tempDir}, Out) {
		slog.Debug("сервер WordPool недоступен, работаем локально")
		err = conv.FilesToPdf(ctx, []string{tempDir}, Out)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка конвертации", slog.Any("err", err))
			os.Exit(1)
		}
	}

	if UseCache {
		if _, err = cache.CommitCache(sources, nil, false); err != nil {
			slog.Error("сохранить кэш", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}
}

// tryServerJob отправляет задание запущенному серверу. Возвращает true, если сервер ответил.
func tryServerJob(port int, sources []string, outDir string) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()

	req := jobRequest{Sources: sources, OutDir: outDir}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		slog.Debug("отправка задания серверу", slog.String("err", err.Error()))
		return false
	}

	var resp jobResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		slog.Debug("чтение ответа сервера", slog.String("err", err.Error()))
		return false
	}

	if resp.Error != "" {
		slog.Error("сервер: " + resp.Error)
		os.Exit(1)
	}
	return true
}

// sendShutdown отправляет серверу команду на остановку.
func sendShutdown(port int) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(jobRequest{Shutdown: true}); err != nil {
		return err
	}

	var resp jobResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

// runServer запускает TCP-сервер с постоянным WordPool.
func runServer(port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("запуск сервера", slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("сервер WordPool запущен", slog.String("addr", addr))

	pool := conv.NewWordPool(4)
	defer pool.Close()

	// Канал для graceful shutdown.
	shutdownCh := make(chan struct{})

	// Сигналы OS.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			slog.Info("сервер остановлен по сигналу")
		case <-shutdownCh:
			slog.Info("сервер остановлен по команде")
		}
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		go handleConn(conn, pool, ln, shutdownCh)
	}

	// Ждём завершения активных соединений.
	slog.Info("ожидание завершения активных заданий...")
	time.Sleep(500 * time.Millisecond)
}

// handleConn обрабатывает одно TCP-соединение.
func handleConn(conn net.Conn, pool *conv.WordPool, ln net.Listener, shutdownCh chan struct{}) {
	defer conn.Close()

	var req jobRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		slog.Error("ошибка парсинга запроса", slog.String("err", err.Error()))
		json.NewEncoder(conn).Encode(jobResponse{Error: err.Error()})
		return
	}

	if req.Shutdown {
		json.NewEncoder(conn).Encode(jobResponse{})
		close(shutdownCh)
		return
	}

	if len(req.Sources) == 0 || req.OutDir == "" {
		json.NewEncoder(conn).Encode(jobResponse{Error: "требуются sources и outDir"})
		return
	}

	slog.Info("конвертация", slog.Any("sources", req.Sources), slog.String("outDir", req.OutDir))

	err := conv.FilesToPdfWithPool(context.Background(), pool, req.Sources, req.OutDir)
	resp := jobResponse{}
	if err != nil {
		resp.Error = err.Error()
		slog.Error("конвертация", slog.String("err", err.Error()))
	}
	json.NewEncoder(conn).Encode(resp)
}

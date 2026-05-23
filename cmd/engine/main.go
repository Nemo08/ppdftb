//go:build windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"log/slog"

	acadpool "github.com/Nemo08/ppdftb/pkg/acadpool"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

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
	var Level string
	var Version, Serve, Shutdown bool
	var Port int

	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&Serve, "serve", false, "режим сервера")
	flag.BoolVar(&Shutdown, "shutdown", false, "остановить сервер")
	flag.IntVar(&Port, "port", defaultPort, "порт TCP-сервера")

	flag.Parse()

	slogutil.Setup(Level)

	if Version {
		fmt.Println("ppdftb engine")
		return
	}

	if Serve {
		runServer(Port)
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

func runServer(port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("engine запущен", slog.String("addr", addr))

	acadPool := acadpool.NewAcadPool(1)
	defer acadPool.Close()

	exeDir := getExeDir()

	shutdownCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			slog.Info("остановка по сигналу")
		case <-shutdownCh:
			slog.Info("остановка по команде")
		}
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		go handleConn(conn, acadPool, exeDir, shutdownCh)
	}

	slog.Info("ожидание завершения заданий...")
	time.Sleep(500 * time.Millisecond)
}

func handleConn(conn net.Conn, acadPool *acadpool.AcadPool, exeDir string, shutdownCh chan struct{}) {
	defer conn.Close()

	var req jobRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(jobResponse{Error: err.Error()})
		return
	}

	slog.Info("задание", slog.String("tool", req.Tool), slog.Any("args", req.Args))

	var runErr error
	switch req.Tool {
	case "wconv":
		runErr = runWconv(req.Args)
	case "aconv":
		runErr = runAconv(acadPool, req.Args)
	case "toc":
		runErr = execTool(exeDir, "toc", req.Args)
	case "mpdf":
		runErr = execTool(exeDir, "mpdf", req.Args)
	case "pnpdf":
		runErr = execTool(exeDir, "pnpdf", req.Args)
	default:
		runErr = fmt.Errorf("неизвестная утилита: %s", req.Tool)
	}

	resp := jobResponse{}
	if runErr != nil {
		resp.Error = runErr.Error()
		slog.Error("ошибка выполнения", slog.String("tool", req.Tool), slog.String("err", runErr.Error()))
	}
	json.NewEncoder(conn).Encode(resp)
}

func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func execTool(exeDir, tool string, args []string) error {
	cmd := exec.Command(filepath.Join(exeDir, tool+".exe"), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runWconv — логика wconv.
func runWconv(args []string) error {
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
	}
	return conv.RunWconv(context.Background(), p)
}

// runAconv — логика aconv с переданным AcadPool.
func runAconv(pool *acadpool.AcadPool, args []string) error {
	fs := flag.NewFlagSet("aconv", flag.ContinueOnError)
	var SrcFile, SrcDir, Out string

	fs.StringVar(&SrcFile, "sf", "", "")
	fs.StringVar(&SrcDir, "sd", "", "")
	fs.StringVar(&Out, "o", "", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	Out = strings.TrimRight(Out, `/\`)
	if Out == "" {
		return fmt.Errorf("Должна быть указана папка для PDF (-o)")
	}

	ctx := context.Background()

	inputCadFiles := conv.CollectCadFiles(SrcFile, SrcDir)
	if len(inputCadFiles) == 0 {
		slog.Info("Нет DWG/DXF файлов для конвертации")
		return nil
	}

	return conv.A2pdfWithPool(ctx, pool, inputCadFiles, Out)
}

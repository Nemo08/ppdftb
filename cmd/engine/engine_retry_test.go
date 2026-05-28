//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// TestSendRequestRetry проверяет что sendRequest повторяет подключение
// пока сервер не станет доступен (симулируем задержку старта).
func TestSendRequestRetry(t *testing.T) {
	// Выбираем свободный порт.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // освобождаем — сервер поднимется с задержкой

	var serverStarted atomic.Bool

	// Поднимаем сервер через 300ms.
	go func() {
		time.Sleep(300 * time.Millisecond)
		srv, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return
		}
		serverStarted.Store(true)
		conn, err := srv.Accept()
		if err != nil {
			_ = srv.Close()
			return
		}
		defer func() { _ = srv.Close() }()
		defer func() { _ = conn.Close() }()
		var req jobRequest
		_ = json.NewDecoder(conn).Decode(&req)
		_ = json.NewEncoder(conn).Encode(jobResponse{})
	}()

	start := time.Now()
	err = sendRequest(port, "mpdf", []string{"-d", "."})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("sendRequest вернул ошибку: %v", err)
	}
	if !serverStarted.Load() {
		t.Error("сервер не был запущен к моменту успешного подключения")
	}
	if elapsed < 200*time.Millisecond {
		t.Errorf("слишком быстрое подключение (%v) — retry не работает", elapsed)
	}
	t.Logf("подключился за %v", elapsed)
}

// TestSendRequestNoServer проверяет что sendRequest возвращает ошибку
// если сервер так и не поднялся (исчерпаны все попытки).
func TestSendRequestNoServer(t *testing.T) {
	// Берём порт на котором точно ничего нет.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	// Временно переопределяем константы retry через замену в тесте невозможна —
	// проверяем через очень короткий контекст (не передаётся в sendRequest,
	// поэтому просто убеждаемся что функция завершается за разумное время).
	done := make(chan error, 1)
	go func() {
		done <- sendRequest(port, "mpdf", nil)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("ожидали ошибку при отсутствии сервера")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sendRequest завис — не завершился за 10 секунд")
	}
}

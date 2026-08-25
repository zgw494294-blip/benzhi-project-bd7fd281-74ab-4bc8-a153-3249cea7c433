package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/store/sqlite"
	httptransport "bioacoustic-release-hub/internal/transport/http"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"level": "error", "message": err.Error()})
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("初始化仓储: %w", err)
	}
	defer store.Close()
	service := application.NewService(store)
	handler := httptransport.NewHandler(service)
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.Addr, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	serveError := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveError <- err
		}
		close(serveError)
	}()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"level": "info", "event": "server.started", "addr": listener.Addr().String(), "selfcheck": cfg.SelfCheck})
	if cfg.SelfCheck {
		return runBoundedSelfCheck(server, serveError, listener.Addr().String())
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"level": "info", "event": "server.stopping", "signal": sig.String()})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-serveError:
		return err
	}
}

func runBoundedSelfCheck(server *http.Server, serveError <-chan error, addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	checkDone := make(chan error, 1)
	go func() { checkDone <- httptransport.RunSelfCheck(ctx, "http://"+addr) }()
	var result error
	select {
	case result = <-checkDone:
	case err := <-serveError:
		if err == nil {
			result = fmt.Errorf("服务在自检完成前退出")
		} else {
			result = err
		}
	case <-ctx.Done():
		result = fmt.Errorf("selfcheck 超时: %w", ctx.Err())
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if result != nil {
		return result
	}
	if shutdownErr != nil {
		return fmt.Errorf("关闭自检服务: %w", shutdownErr)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"level": "info", "event": "selfcheck.passed"})
	return nil
}

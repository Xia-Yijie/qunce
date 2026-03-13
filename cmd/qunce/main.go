package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	clientapp "qunce/cmd/qunce/clientapp"
)

func main() {
	if err := runApp(); err != nil {
		log.Fatal(err)
	}
}

func runApp() error {
	mode, args := parseMode(os.Args[1:])
	switch mode {
	case "server":
		return runServer()
	case "client":
		return clientapp.Run(args)
	case "help":
		_, _ = os.Stdout.WriteString("usage: qunce [server|client] [options]\n  qunce server   start server (default)\n  qunce client   start agent client\n")
		return nil
	default:
		return fmt.Errorf("unknown mode %q, use `qunce server` or `qunce client`", mode)
	}
}

func parseMode(raw []string) (string, []string) {
	if len(raw) == 0 {
		return "server", nil
	}
	if !strings.HasPrefix(raw[0], "-") {
		switch raw[0] {
		case "server", "client", "help":
			return raw[0], raw[1:]
		}
	}
	return "server", raw
}

func runServer() error {
	cfg := loadConfig()
	st, err := newState(cfg)
	if err != nil {
		return err
	}

	agentRegistry := newAgentRegistry()
	consoleRegistry := newConsoleRegistry()
	ctx := newHandlerContext(st, agentRegistry, consoleRegistry, cfg)

	mux := http.NewServeMux()
	registerRoutes(mux, ctx)
	registerAgentSocket(mux, st, agentRegistry, consoleRegistry, cfg)
	registerConsoleSocket(mux, consoleRegistry, st)
	registerStaticRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("qunce server listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()

	serverCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runEmbeddedNode(serverCtx, cfg)
	}()

	select {
	case err := <-errCh:
		stop()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		wg.Wait()
		return nil
	case <-serverCtx.Done():
		log.Printf("shutdown requested")
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := server.Shutdown(ctxTimeout)
		wg.Wait()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

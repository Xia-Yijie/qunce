package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type endpointRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type rawEnvelope struct {
	V         int             `json:"v"`
	Type      string          `json:"type"`
	EventID   string          `json:"event_id"`
	RequestID string          `json:"request_id,omitempty"`
	TS        string          `json:"ts"`
	Source    endpointRef     `json:"source"`
	Target    endpointRef     `json:"target"`
	Data      json.RawMessage `json:"data"`
}

type agentConfig struct {
	ServerURL  string
	ServerAddr string
	PairToken  string
	NodeID     string
	NodeName   string
	MaxWorker  int
	WorkDir    string
	Store      *localStore
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		printUsage()
		os.Exit(2)
	}
	defer cfg.Store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, cfg); err != nil {
		logger.Error("agent exited with error", "error", err)
		os.Exit(1)
	}
}

func loadConfig(args []string) (agentConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "local-agent"
	}

	if len(args) < 1 || args[0] == "" {
		return agentConfig{}, errors.New("missing server address, expected host:port")
	}

	serverURL, serverAddr, err := normalizeServerAddress(args[0])
	if err != nil {
		return agentConfig{}, err
	}

	workDir, err := resolveWorkDir(args)
	if err != nil {
		return agentConfig{}, err
	}

	store, err := openLocalStore(workDir)
	if err != nil {
		return agentConfig{}, err
	}

	pairToken := firstNonEmpty(
		os.Getenv("QUNCE_PAIR_TOKEN"),
		mustGetConfig(store, "pair_token"),
		"dev-pair-token",
	)
	nodeName := firstNonEmpty(
		os.Getenv("QUNCE_NODE_NAME"),
		mustGetConfig(store, "node_name"),
		hostname,
	)
	nodeID := firstNonEmpty(
		os.Getenv("QUNCE_NODE_ID"),
		mustGetConfig(store, "node_id"),
		generateNodeID(),
	)

	if err := persistBootConfig(store, serverAddr, pairToken, nodeID, nodeName, workDir); err != nil {
		store.Close()
		return agentConfig{}, err
	}

	return agentConfig{
		ServerURL:  serverURL,
		ServerAddr: serverAddr,
		PairToken:  pairToken,
		NodeID:     nodeID,
		NodeName:   nodeName,
		MaxWorker:  2,
		WorkDir:    workDir,
		Store:      store,
	}, nil
}

func run(ctx context.Context, logger *slog.Logger, cfg agentConfig) error {
	conn, _, err := websocket.Dial(ctx, cfg.ServerURL, nil)
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "shutdown")

	logger.Info("connected to server", "url", cfg.ServerURL, "workdir", cfg.WorkDir)
	_ = cfg.Store.RecordEvent("agent.connected", cfg.ServerURL)

	if err := sendEnvelope(ctx, conn, buildEnvelope("agent.hello", cfg.NodeID, map[string]any{
		"hostname":      cfg.NodeName,
		"platform":      runtimePlatform(),
		"arch":          runtimeArch(),
		"agent_version": "0.1.0",
		"session_id":    nil,
	})); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	if _, err := readEnvelope(ctx, conn); err != nil {
		return fmt.Errorf("read server hello: %w", err)
	}

	if err := sendEnvelope(ctx, conn, buildEnvelope("agent.auth", cfg.NodeID, map[string]any{
		"pair_token": cfg.PairToken,
		"node_id":    cfg.NodeID,
	})); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	authEnvelope, err := readEnvelope(ctx, conn)
	if err != nil {
		return fmt.Errorf("read auth reply: %w", err)
	}
	logger.Info("received auth reply", "type", authEnvelope.Type)
	_ = cfg.Store.RecordEvent(authEnvelope.Type, string(authEnvelope.Data))

	if authEnvelope.Type != "server.auth.ok" {
		return fmt.Errorf("auth rejected: %s", authEnvelope.Type)
	}

	authPayload, err := parseAuthReply(authEnvelope.Data)
	if err != nil {
		return fmt.Errorf("decode auth reply: %w", err)
	}
	if authPayload.NodeID != "" {
		cfg.NodeID = authPayload.NodeID
	}
	if authPayload.NodeName != "" {
		cfg.NodeName = authPayload.NodeName
	}
	if err := persistBootConfig(cfg.Store, cfg.ServerAddr, cfg.PairToken, cfg.NodeID, cfg.NodeName, cfg.WorkDir); err != nil {
		return err
	}

	if err := sendEnvelope(ctx, conn, buildEnvelope("agent.state.report", cfg.NodeID, map[string]any{
		"status":        "online",
		"max_workers":   cfg.MaxWorker,
		"running_turns": []any{},
	})); err != nil {
		return fmt.Errorf("report state: %w", err)
	}

	go func() {
		for {
			message, readErr := readEnvelope(ctx, conn)
			if readErr != nil {
				logger.Warn("read loop stopped", "error", readErr)
				return
			}
			logger.Info("received message", "type", message.Type)
			_ = cfg.Store.RecordEvent(message.Type, string(message.Data))
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down agent")
			return nil
		case <-ticker.C:
			if err := sendEnvelope(ctx, conn, buildEnvelope("agent.ping", cfg.NodeID, map[string]any{
				"running_turns":          0,
				"worker_count":           0,
				"load":                   0.0,
				"last_completed_turn_id": nil,
			})); err != nil {
				return fmt.Errorf("send ping: %w", err)
			}
		}
	}
}

func buildEnvelope(messageType, nodeID string, data map[string]any) map[string]any {
	targetID := "main"
	if nodeID == "" {
		nodeID = "unbound"
	}

	return map[string]any{
		"v":          1,
		"type":       messageType,
		"event_id":   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		"request_id": fmt.Sprintf("req_%d", time.Now().UnixNano()),
		"ts":         time.Now().UTC().Format(time.RFC3339),
		"source": map[string]any{
			"kind": "agent",
			"id":   nodeID,
		},
		"target": map[string]any{
			"kind": "server",
			"id":   targetID,
		},
		"data": data,
	}
}

func sendEnvelope(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	return wsjson.Write(ctx, conn, payload)
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (rawEnvelope, error) {
	var env rawEnvelope
	err := wsjson.Read(ctx, conn, &env)
	return env, err
}

func normalizeServerAddress(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", errors.New("server address is empty")
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("invalid server address: %w", err)
		}
		if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
			return "", "", fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			return "", "", errors.New("server address missing host:port")
		}
		if parsed.Path == "" || parsed.Path == "/" {
			parsed.Path = "/ws/agent"
		}
		return parsed.String(), parsed.Host, nil
	}

	withScheme := trimmed
	if !strings.HasPrefix(withScheme, "ws://") && !strings.HasPrefix(withScheme, "wss://") {
		withScheme = "ws://" + withScheme
	}

	parsed, err := url.Parse(withScheme)
	if err != nil {
		return "", "", fmt.Errorf("invalid server address: %w", err)
	}
	if parsed.Host == "" {
		return "", "", errors.New("server address missing host:port")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/ws/agent"
	}

	return parsed.String(), parsed.Host, nil
}

func resolveWorkDir(args []string) (string, error) {
	if len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
		return expandHome(args[1])
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	return filepath.Join(home, ".qunce"), nil
}

func expandHome(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("workdir is empty")
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		if trimmed == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(trimmed, "~/")), nil
	}
	return filepath.Clean(trimmed), nil
}

func persistBootConfig(store *localStore, serverAddr, pairToken, nodeID, nodeName, workDir string) error {
	for key, value := range map[string]string{
		"server_addr": serverAddr,
		"pair_token":  pairToken,
		"node_id":     nodeID,
		"node_name":   nodeName,
		"work_dir":    workDir,
	} {
		if err := store.Set(key, value); err != nil {
			return err
		}
	}
	return nil
}

func mustGetConfig(store *localStore, key string) string {
	value, err := store.Get(key)
	if err != nil {
		return ""
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func generateNodeID() string {
	buffer := make([]byte, 6)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return fmt.Sprintf("node_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("node_%x", buffer)
}

type authReplyData struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
}

func parseAuthReply(data json.RawMessage) (authReplyData, error) {
	if len(data) == 0 || string(data) == "null" {
		return authReplyData{}, nil
	}

	var payload authReplyData
	if err := json.Unmarshal(data, &payload); err != nil {
		return authReplyData{}, err
	}
	return payload, nil
}

func runtimePlatform() string {
	return runtime.GOOS
}

func runtimeArch() string {
	return runtime.GOARCH
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: agentd <host:port|ws://host:port/ws/agent> [workdir]")
	fmt.Fprintln(os.Stderr, "example: agentd 127.0.0.1:8000 ~/.qunce")
}

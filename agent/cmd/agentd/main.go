package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"qunce-agent/pkg/agentcore"
)

type agentConfig struct {
	ServerURL  string
	ServerAddr string
	PairToken  string
	NodeID     string
	NodeName   string
	Hostname   string
	Hello      string
	MaxWorker  int
	WorkDir    string
	Store      *agentcore.LocalStore
}

type agentRuntime struct {
	conn     *websocket.Conn
	cfg      agentConfig
	logger   *slog.Logger
	sendLock sync.Mutex
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
	defaultNodeName := agentcore.FirstNonEmpty(os.Getenv("QUNCE_NODE_NAME"), os.Getenv("USER"), os.Getenv("USERNAME"), "agent") + "@" + hostname

	fs := flag.NewFlagSet("agentd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	link := fs.String("link", "", "server host:port")
	workspace := fs.String("workspace", "~/.qunce", "agent workspace")
	hello := fs.String("hello", "", "greeting message")
	if err := fs.Parse(args); err != nil {
		return agentConfig{}, err
	}
	if fs.NArg() > 0 {
		return agentConfig{}, errors.New("unexpected positional arguments, use --link/--workspace/--hello")
	}
	if strings.TrimSpace(*link) == "" {
		return agentConfig{}, errors.New("missing --link, expected host:port")
	}
	if strings.Contains(*link, "://") {
		return agentConfig{}, errors.New("--link must use host:port only, do not include ws://")
	}

	serverURL, serverAddr, err := agentcore.NormalizeServerAddress(*link)
	if err != nil {
		return agentConfig{}, err
	}

	workDir, err := agentcore.ExpandHome(*workspace)
	if err != nil {
		return agentConfig{}, err
	}

	store, err := agentcore.OpenLocalStore(workDir)
	if err != nil {
		return agentConfig{}, err
	}

	pairToken := agentcore.FirstNonEmpty(
		os.Getenv("QUNCE_PAIR_TOKEN"),
		agentcore.MustGetConfig(store, "pair_token"),
		"dev-pair-token",
	)
	nodeName := agentcore.FirstNonEmpty(
		os.Getenv("QUNCE_NODE_NAME"),
		defaultNodeName,
		agentcore.MustGetConfig(store, "node_name"),
	)
	helloMessage := agentcore.FirstNonEmpty(
		strings.TrimSpace(*hello),
		agentcore.MustGetConfig(store, "hello_message"),
		"你好，我想加入群策。",
	)
	nodeID := agentcore.FirstNonEmpty(
		os.Getenv("QUNCE_NODE_ID"),
		agentcore.MustGetConfig(store, "node_id"),
		agentcore.GenerateNodeID(),
	)

	if err := agentcore.PersistBootConfig(store, serverAddr, pairToken, nodeID, nodeName, workDir, helloMessage); err != nil {
		store.Close()
		return agentConfig{}, err
	}

	return agentConfig{
		ServerURL:  serverURL,
		ServerAddr: serverAddr,
		PairToken:  pairToken,
		NodeID:     nodeID,
		NodeName:   nodeName,
		Hostname:   hostname,
		Hello:      helloMessage,
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

	runtime := &agentRuntime{conn: conn, cfg: cfg, logger: logger}

	if err := runtime.send(ctx, agentcore.BuildEnvelope("agent.hello", cfg.NodeID, map[string]any{
		"username":      cfg.NodeName,
		"hostname":      cfg.Hostname,
		"hello_message": cfg.Hello,
		"work_dir":      cfg.WorkDir,
		"platform":      goruntime.GOOS,
		"arch":          goruntime.GOARCH,
		"agent_version": "0.1.0",
		"session_id":    nil,
	})); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	if _, err := agentcore.ReadEnvelope(ctx, conn); err != nil {
		return fmt.Errorf("read server hello: %w", err)
	}

	if err := runtime.send(ctx, agentcore.BuildEnvelope("agent.auth", cfg.NodeID, map[string]any{
		"pair_token": cfg.PairToken,
		"node_id":    cfg.NodeID,
	})); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	authEnvelope, err := agentcore.ReadEnvelope(ctx, conn)
	if err != nil {
		return fmt.Errorf("read auth reply: %w", err)
	}
	logger.Info("received auth reply", "type", authEnvelope.Type)
	_ = cfg.Store.RecordEvent(authEnvelope.Type, string(authEnvelope.Data))

	if authEnvelope.Type != "server.auth.ok" {
		return fmt.Errorf("auth rejected: %s", authEnvelope.Type)
	}

	authPayload, err := agentcore.ParseAuthReply(authEnvelope.Data)
	if err != nil {
		return fmt.Errorf("decode auth reply: %w", err)
	}
	if authPayload.NodeID != "" {
		cfg.NodeID = authPayload.NodeID
	}
	if authPayload.NodeName != "" {
		cfg.NodeName = authPayload.NodeName
	}
	if err := agentcore.PersistBootConfig(cfg.Store, cfg.ServerAddr, cfg.PairToken, cfg.NodeID, cfg.NodeName, cfg.WorkDir, cfg.Hello); err != nil {
		return err
	}

	if err := runtime.send(ctx, agentcore.BuildEnvelope("agent.state.report", cfg.NodeID, map[string]any{
		"status":           "online",
		"max_workers":      cfg.MaxWorker,
		"worker_count":     0,
		"running_turn_ids": []string{},
	})); err != nil {
		return fmt.Errorf("report state: %w", err)
	}

	readErrCh := make(chan error, 1)
	go func() {
		for {
			message, readErr := agentcore.ReadEnvelope(ctx, conn)
			if readErr != nil {
				logger.Warn("read loop stopped", "error", readErr)
				select {
				case readErrCh <- readErr:
				default:
				}
				return
			}
			logger.Info("received message", "type", message.Type)
			_ = cfg.Store.RecordEvent(message.Type, string(message.Data))
			if message.Type == "server.turn.request" {
				go runtime.handleTurnRequest(ctx, message)
				continue
			}
			if message.Type == "server.workspace.validate" {
				go runtime.handleWorkspaceValidation(ctx, message)
			}
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down agent")
			return nil
		case readErr := <-readErrCh:
			return fmt.Errorf("connection closed: %w", readErr)
		case <-ticker.C:
			if err := runtime.send(ctx, agentcore.BuildEnvelope("agent.ping", cfg.NodeID, map[string]any{
				"running_turn_ids":       []string{},
				"worker_count":           0,
				"load":                   0.0,
				"last_completed_turn_id": nil,
			})); err != nil {
				return fmt.Errorf("send ping: %w", err)
			}
		}
	}
}

type turnRequestData struct {
	TurnID       string `json:"turn_id"`
	ChatID       string `json:"chat_id"`
	MessageID    string `json:"message_id"`
	Content      string `json:"content"`
	SenderName   string `json:"sender_name"`
	PersonaID    string `json:"persona_id"`
	PersonaName  string `json:"persona_name"`
	WorkspaceDir string `json:"workspace_dir"`
	SystemPrompt string `json:"system_prompt"`
	AgentKey     string `json:"agent_key"`
	AgentLabel   string `json:"agent_label"`
	Muted        bool   `json:"muted"`
}

type workspaceValidationRequest struct {
	WorkspaceDir string `json:"workspace_dir"`
}

func (r *agentRuntime) handleTurnRequest(ctx context.Context, message agentcore.RawEnvelope) {
	var data turnRequestData
	if err := json.Unmarshal(message.Data, &data); err != nil {
		r.logger.Warn("failed to decode turn request", "error", err)
		return
	}

	if data.TurnID == "" {
		return
	}

	if err := r.send(ctx, agentcore.BuildEnvelope("agent.turn.read", r.cfg.NodeID, map[string]any{
		"turn_id":          data.TurnID,
		"worker_count":     0,
		"running_turn_ids": []string{},
	})); err != nil {
		r.logger.Warn("failed to send turn read", "error", err, "turn_id", data.TurnID)
		return
	}

	if data.Muted {
		return
	}

	if err := r.send(ctx, agentcore.BuildEnvelope("agent.turn.started", r.cfg.NodeID, map[string]any{
		"turn_id":          data.TurnID,
		"worker_count":     1,
		"running_turn_ids": []string{data.TurnID},
	})); err != nil {
		r.logger.Warn("failed to send turn started", "error", err, "turn_id", data.TurnID)
		return
	}

	time.Sleep(900 * time.Millisecond)
	output := fmt.Sprintf("已处理你的请求：%s", data.Content)
	if err := r.send(ctx, agentcore.BuildEnvelope("agent.turn.completed", r.cfg.NodeID, map[string]any{
		"turn_id":          data.TurnID,
		"output":           output,
		"worker_count":     0,
		"running_turn_ids": []string{},
	})); err != nil {
		r.logger.Warn("failed to send turn completed", "error", err, "turn_id", data.TurnID)
	}
}

func (r *agentRuntime) handleWorkspaceValidation(ctx context.Context, message agentcore.RawEnvelope) {
	var data workspaceValidationRequest
	if err := json.Unmarshal(message.Data, &data); err != nil {
		r.logger.Warn("failed to decode workspace validation request", "error", err)
		return
	}

	ok, normalizedPath, detail := agentcore.ValidateWorkspacePath(strings.TrimSpace(data.WorkspaceDir))
	if err := r.send(
		ctx,
		agentcore.BuildReplyEnvelope(
			"agent.workspace.validated",
			r.cfg.NodeID,
			message.RequestID,
			map[string]any{
				"ok":              ok,
				"normalized_path": normalizedPath,
				"message":         detail,
			},
		),
	); err != nil {
		r.logger.Warn("failed to send workspace validation result", "error", err)
	}
}

func (r *agentRuntime) send(ctx context.Context, payload map[string]any) error {
	r.sendLock.Lock()
	defer r.sendLock.Unlock()
	return agentcore.SendEnvelope(ctx, r.conn, payload)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: agentd --link 127.0.0.1:8000 [--workspace ~/.qunce] [--hello \"你好，我想加入群策。\"]")
}

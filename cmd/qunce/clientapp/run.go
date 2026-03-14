package clientapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
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

func Run(args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := loadConfig(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		printUsage()
		return err
	}
	defer cfg.Store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, cfg); err != nil {
		logger.Error("agent exited with error", "error", err)
		return err
	}
	return nil
}

func loadConfig(args []string) (agentConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "local-agent"
	}
	defaultNodeName := agentcore.FirstNonEmpty(os.Getenv("QUNCE_NODE_NAME"), os.Getenv("USER"), os.Getenv("USERNAME"), "agent") + "@" + hostname

	fs := flag.NewFlagSet("qunce-client", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	link := fs.String("link", "", "server host:port")
	workspace := fs.String("workspace", "~/.qunce", "agent workspace")
	hello := fs.String("hello", "", "greeting message")
	nodeIDArg := fs.String("node-id", "", "agent node id")
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
		strings.TrimSpace(*nodeIDArg),
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

	output, execErr := r.generateTurnReply(ctx, data)
	if execErr != nil {
		r.logger.Warn(
			"failed to generate turn reply",
			"error",
			execErr,
			"turn_id",
			data.TurnID,
			"workspace_dir",
			data.WorkspaceDir,
		)
	}
	if err := r.send(ctx, agentcore.BuildEnvelope("agent.turn.completed", r.cfg.NodeID, map[string]any{
		"turn_id":          data.TurnID,
		"output":           output,
		"worker_count":     0,
		"running_turn_ids": []string{},
	})); err != nil {
		r.logger.Warn("failed to send turn completed", "error", err, "turn_id", data.TurnID)
	}
}

func (r *agentRuntime) generateTurnReply(ctx context.Context, data turnRequestData) (string, error) {
	workspaceDir := strings.TrimSpace(data.WorkspaceDir)
	if workspaceDir == "" {
		workspaceDir = r.cfg.WorkDir
	}
	if workspaceDir == "" {
		return "I could not reply because no workspace directory is configured.", errors.New("workspace directory is empty")
	}

	info, err := os.Stat(workspaceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.MkdirAll(workspaceDir, 0o755); mkErr != nil {
				return fmt.Sprintf("I could not create the workspace directory: %s", workspaceDir), mkErr
			}
			info, err = os.Stat(workspaceDir)
		}
		if err != nil {
			return fmt.Sprintf("I could not reply because the workspace is unavailable: %s", workspaceDir), err
		}
	}
	if !info.IsDir() {
		return fmt.Sprintf("I could not reply because the workspace path is not a directory: %s", workspaceDir), errors.New("workspace path is not a directory")
	}

	prompt := buildTurnPrompt(data, workspaceDir)
	output, err := runCodexExec(ctx, workspaceDir, prompt)
	if err != nil {
		return fmt.Sprintf("I could not finish the reply in %s: %v", workspaceDir, err), err
	}
	output = strings.TrimSpace(output)
	r.logger.Info("generated turn reply", "turn_id", data.TurnID, "output", output)
	if output == "" {
		return "I could not produce a reply.", errors.New("empty codex output")
	}
	return output, nil
}

func buildTurnPrompt(data turnRequestData, workspaceDir string) string {
	var builder strings.Builder
	builder.WriteString("You are replying as one participant inside a multi-agent group chat.\n")
	builder.WriteString("Return only the final message that should be posted to the chat.\n")
	builder.WriteString("Reply in Chinese unless the user clearly used another language.\n")
	builder.WriteString("Do not include analysis, tool logs, markdown fences, or meta commentary.\n")
	builder.WriteString("Do not say that you received the request.\n")
	builder.WriteString("Do not paraphrase or repeat the user's message unless quoting is necessary.\n")
	builder.WriteString("If the user is greeting you, greet back naturally and briefly.\n")
	builder.WriteString("If the user asks a question, answer it directly.\n")
	builder.WriteString("Do not modify files.\n")
	builder.WriteString("Keep the reply concise and useful.\n\n")
	builder.WriteString("Context:\n")
	builder.WriteString("Persona name: ")
	builder.WriteString(agentcore.FirstNonEmpty(strings.TrimSpace(data.PersonaName), strings.TrimSpace(data.PersonaID), "Agent"))
	builder.WriteString("\n")
	if prompt := strings.TrimSpace(data.SystemPrompt); prompt != "" {
		builder.WriteString("System prompt:\n")
		builder.WriteString(prompt)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Workspace directory: ")
	builder.WriteString(workspaceDir)
	builder.WriteString("\n")
	builder.WriteString("User name: ")
	builder.WriteString(agentcore.FirstNonEmpty(strings.TrimSpace(data.SenderName), "user"))
	builder.WriteString("\n")
	builder.WriteString("User message:\n")
	builder.WriteString(strings.TrimSpace(data.Content))
	builder.WriteString("\n\n")
	builder.WriteString("Now write the chat reply.\n")
	return builder.String()
}

func runCodexExec(ctx context.Context, workspaceDir, prompt string) (string, error) {
	outputFile, err := os.CreateTemp("", "qunce-codex-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp output file: %w", err)
	}
	outputPath := outputFile.Name()
	if closeErr := outputFile.Close(); closeErr != nil {
		_ = os.Remove(outputPath)
		return "", fmt.Errorf("prepare temp output file: %w", closeErr)
	}
	defer os.Remove(outputPath)

	execCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--color", "never",
		"-C", workspaceDir,
		"--output-last-message", outputPath,
		"-",
	}
	cmd, err := buildCodexCommand(execCtx, args...)
	if err != nil {
		return "", err
	}
	cmd.Dir = workspaceDir
	cmd.Stdin = strings.NewReader(prompt)
	configureChildProcess(cmd)

	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if output, readErr := readCodexOutputFile(outputPath); readErr == nil && output != "" {
			return output, nil
		}
		errText := strings.TrimSpace(stderr.String())
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("codex timed out after 2 minutes")
		}
		if errText == "" {
			errText = err.Error()
		}
		return "", fmt.Errorf("codex exec failed: %s", errText)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read codex output: %w", err)
	}
	return string(raw), nil
}

func readCodexOutputFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func buildCodexCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	for _, candidate := range defaultCodexCommandCandidates() {
		if resolved, err := exec.LookPath(candidate); err == nil {
			return exec.CommandContext(ctx, resolved, args...), nil
		}
	}

	return nil, errors.New("codex executable not found")
}

func defaultCodexCommandCandidates() []string {
	return []string{"codex"}
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
	fmt.Fprintln(
		os.Stderr,
		`usage: qunce client --link 127.0.0.1:8000 [--workspace ~/.qunce] [--hello "你好，我想加入群策。"]`,
	)
}

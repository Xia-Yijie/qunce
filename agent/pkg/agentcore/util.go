package agentcore

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type EndpointRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type RawEnvelope struct {
	V         int             `json:"v"`
	Type      string          `json:"type"`
	EventID   string          `json:"event_id"`
	RequestID string          `json:"request_id,omitempty"`
	TS        string          `json:"ts"`
	Source    EndpointRef     `json:"source"`
	Target    EndpointRef     `json:"target"`
	Data      json.RawMessage `json:"data"`
}

type AuthReplyData struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
}

func ParseAuthReply(data json.RawMessage) (AuthReplyData, error) {
	if len(data) == 0 || string(data) == "null" {
		return AuthReplyData{}, nil
	}

	var payload AuthReplyData
	if err := json.Unmarshal(data, &payload); err != nil {
		return AuthReplyData{}, err
	}
	return payload, nil
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func GenerateNodeID() string {
	buffer := make([]byte, 6)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return fmt.Sprintf("node_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("node_%x", buffer)
}

func ExpandHome(path string) (string, error) {
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

func NormalizeServerAddress(raw string) (string, string, error) {
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

func ValidateWorkspacePath(raw string) (bool, string, string) {
	if raw == "" {
		return false, "", "工作目录不能为空"
	}

	normalized := filepath.Clean(raw)
	if !filepath.IsAbs(normalized) {
		return false, normalized, "工作目录必须是绝对路径"
	}

	info, err := os.Stat(normalized)
	if err == nil {
		if !info.IsDir() {
			return false, normalized, "该路径已存在，但不是目录"
		}
		entries, readErr := os.ReadDir(normalized)
		if readErr != nil {
			return false, normalized, "目录不可读取，请检查权限"
		}
		if len(entries) > 0 {
			return false, normalized, "目录必须为空"
		}
		if writeErr := verifyDirectoryWritable(normalized); writeErr != nil {
			return false, normalized, "目录不可写，请检查权限"
		}
		return true, normalized, "目录可用：已存在且为空目录"
	}

	if !errors.Is(err, os.ErrNotExist) {
		return false, normalized, "无法访问该目录，请检查权限"
	}

	parent := filepath.Dir(normalized)
	parentInfo, parentErr := os.Stat(parent)
	if parentErr != nil {
		if errors.Is(parentErr, os.ErrNotExist) {
			return false, normalized, "父目录不存在，无法在这里创建"
		}
		return false, normalized, "无法访问父目录，请检查权限"
	}
	if !parentInfo.IsDir() {
		return false, normalized, "父路径不是目录"
	}
	if writeErr := verifyParentCreatable(parent); writeErr != nil {
		return false, normalized, "父目录不可写，无法创建该目录"
	}
	return true, normalized, "目录可用：目标目录不存在，但可以创建"
}

func verifyDirectoryWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".qunce-write-check-*")
	if err != nil {
		return err
	}
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probe.Name())
		return closeErr
	}
	return os.Remove(probe.Name())
}

func verifyParentCreatable(parent string) error {
	probeDir, err := os.MkdirTemp(parent, ".qunce-create-check-*")
	if err != nil {
		return err
	}
	return os.Remove(probeDir)
}

func BuildEnvelope(messageType, nodeID string, data map[string]any) map[string]any {
	return BuildReplyEnvelope(messageType, nodeID, fmt.Sprintf("req_%d", time.Now().UnixNano()), data)
}

func BuildReplyEnvelope(messageType, nodeID, requestID string, data map[string]any) map[string]any {
	targetID := "main"
	if nodeID == "" {
		nodeID = "unbound"
	}

	return map[string]any{
		"v":          1,
		"type":       messageType,
		"event_id":   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		"request_id": requestID,
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

func SendEnvelope(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	return wsjson.Write(ctx, conn, payload)
}

func ReadEnvelope(ctx context.Context, conn *websocket.Conn) (RawEnvelope, error) {
	var env RawEnvelope
	err := wsjson.Read(ctx, conn, &env)
	return env, err
}

func PersistBootConfig(store *LocalStore, serverAddr, pairToken, nodeID, nodeName, workDir, helloMessage string) error {
	for key, value := range map[string]string{
		"server_addr":   serverAddr,
		"pair_token":    pairToken,
		"node_id":       nodeID,
		"node_name":     nodeName,
		"work_dir":      workDir,
		"hello_message": helloMessage,
	} {
		if err := store.Set(key, value); err != nil {
			return err
		}
	}
	return nil
}

func MustGetConfig(store *LocalStore, key string) string {
	value, err := store.Get(key)
	if err != nil {
		return ""
	}
	return value
}

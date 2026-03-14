package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	clientapp "qunce/cmd/qunce/clientapp"
)

const embeddedNodeWorkspaceSuffix = ".qunce-node"

func runEmbeddedNode(ctx context.Context, cfg appConfig) {
	if !shouldRunEmbeddedNode() {
		return
	}

	backoff := 2 * time.Second
	for attempt := 0; ; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := runEmbeddedNodeOnce(cfg); err != nil {
			wait := exponentialWait(attempt, backoff)
			log.Printf("embedded node exited, retrying in %s: %v", wait, err)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
			continue
		}

		select {
		case <-time.After(2 * time.Second):
			// keep trying immediate reconnect after graceful exit
		case <-ctx.Done():
			return
		}
	}
}

func shouldRunEmbeddedNode() bool {
	value := envOrDefault("QUNCE_EMBEDDED_NODE", "true")
	if value == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func runEmbeddedNodeOnce(cfg appConfig) error {
	workspace := strings.TrimSpace(cfg.ServerDataDir)
	if workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			workspace = "."
		} else {
			workspace = home
		}
	}
	workspace = filepath.Clean(filepath.Join(workspace, embeddedNodeWorkspaceSuffix))

	args := []string{
		"--link", fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		"--workspace", workspace,
		"--hello", envOrDefault("QUNCE_SERVER_NODE_HELLO", "群策服务伴生节点"),
	}
	if cfg.EmbeddedNodeID != "" {
		args = append(args, "--node-id", cfg.EmbeddedNodeID)
	}

	return clientapp.Run(args)
}

func exponentialWait(attempt int, base time.Duration) time.Duration {
	wait := base << attempt
	maxWait := 30 * time.Second
	if wait > maxWait {
		wait = maxWait
	}
	if wait < base {
		wait = base
	}
	return wait
}

func envOrDefault(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

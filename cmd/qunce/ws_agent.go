package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var wsAgentUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func registerAgentSocket(mux *http.ServeMux, st *state, agents *agentRegistry, consoles *consoleRegistry, cfg appConfig) {
	mux.HandleFunc("/ws/agent", handleAgentSocket(st, agents, consoles, cfg))
}

func handleAgentSocket(st *state, agents *agentRegistry, consoles *consoleRegistry, cfg appConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsAgentUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade agent socket: %v", err)
			return
		}

		nodeID := "unbound"
		defer func() {
			agents.remove(nodeID)
			node := st.getNode(nodeID)
			if node != nil {
				status := "pending"
				if node.Approved {
					status = "offline"
				}
				st.updateNode(nodeID, map[string]interface{}{
					"status": status,
				})
				broadcastNodeUpdate(consoles, st)
			}
			_ = conn.Close()
		}()

		helloEnvelope, err := readEnvelope(conn)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.error",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "INVALID_HANDSHAKE",
					"message": "expected agent.hello",
				},
				"",
			)))
			return
		}
		if helloEnvelope.Type != "agent.hello" {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.error",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "INVALID_HANDSHAKE",
					"message": "expected agent.hello",
				},
				helloEnvelope.RequestID,
			)))
			return
		}

		helloPayload, err := dataMap(helloEnvelope.Data)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.error",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "INVALID_HANDSHAKE",
					"message": "invalid hello payload",
				},
				helloEnvelope.RequestID,
			)))
			return
		}

		if err := sendEnvelope(conn, buildEnvelope(
			"server.hello",
			"server",
			"main",
			"agent",
			nodeID,
			map[string]interface{}{
				"server_version":   cfg.AppVersion,
				"heartbeat_sec":    15,
				"resume_supported": false,
			},
			helloEnvelope.RequestID,
		)); err != nil {
			return
		}

		authEnvelope, err := readEnvelope(conn)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.auth.reject",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "INVALID_AUTH",
					"message": "missing auth message",
				},
				"",
			)))
			return
		}
		authPayload, err := dataMap(authEnvelope.Data)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.auth.reject",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "INVALID_AUTH",
					"message": "invalid auth payload",
				},
				authEnvelope.RequestID,
			)))
			return
		}

		if authEnvelope.Type != "agent.auth" || toString(authPayload["pair_token"], "") != cfg.DefaultPairToken {
			_ = conn.WriteMessage(websocket.TextMessage, mustJSON(buildEnvelope(
				"server.auth.reject",
				"server",
				"main",
				"agent",
				nodeID,
				map[string]interface{}{
					"code":    "PAIR_TOKEN_INVALID",
					"message": "pair token invalid",
				},
				authEnvelope.RequestID,
			)))
			return
		}

		hostname := strings.TrimSpace(toString(helloPayload["hostname"], "local-node"))
		username := strings.TrimSpace(toString(helloPayload["username"], hostname))
		nodeID = strings.TrimSpace(toString(authPayload["node_id"], ""))
		if nodeID == "" {
			nodeID = "node_" + strings.ReplaceAll(hostname, " ", "-")
		}
		existingNode := st.getNode(nodeID)
		if existingNode == nil {
			existingNode = &nodeRecord{}
		}

		agents.upsert(nodeID, conn)
		st.updateNode(nodeID, map[string]interface{}{
			"node_id":       nodeID,
			"name":          username,
			"hostname":      hostname,
			"work_dir":      firstNonEmpty(toString(helloPayload["work_dir"], ""), existingNode.WorkDir),
			"platform":      firstNonEmpty(toString(helloPayload["platform"], ""), existingNode.Platform),
			"arch":          firstNonEmpty(toString(helloPayload["arch"], ""), existingNode.Arch),
			"agent_version": firstNonEmpty(toString(helloPayload["agent_version"], ""), existingNode.AgentVersion),
			"hello_message": firstNonEmpty(toString(helloPayload["hello_message"], ""), existingNode.HelloMessage),
			"approved":      existingNode.Approved,
			"status":        boolText(existingNode != nil && existingNode.Approved, "online", "pending"),
			"running_turns": 0,
			"worker_count":  0,
		})
		broadcastNodeUpdate(consoles, st)

		if err := sendEnvelope(conn, buildEnvelope(
			"server.auth.ok",
			"server",
			"main",
			"agent",
			nodeID,
			map[string]interface{}{
				"node_id":     nodeID,
				"node_name":   username,
				"max_workers": 2,
			},
			authEnvelope.RequestID,
		)); err != nil {
			return
		}

		_ = dispatchPendingTurnsForNode(st, agents, nodeID)

		sendEnvelopeSafeForNode := func(target *agentConnection, data envelope) error {
			if target == nil {
				return nil
			}
			return sendEnvelopeSafe(target, data)
		}

		for {
			raw, err := readEnvelope(conn)
			if err != nil {
				return
			}
			payload, err := dataMap(raw.Data)
			if err != nil {
				_ = sendEnvelopeSafeForNode(agents.get(nodeID), buildEnvelope(
					"server.notice",
					"server",
					"main",
					"agent",
					nodeID,
					map[string]interface{}{"level": "warning", "message": "invalid payload"},
					raw.RequestID,
				))
				continue
			}

			updateNodeState := func() {
				runningTurns := toInt(payload["running_turns"])
				if runningTurns == 0 {
					if ids, ok := payload["running_turn_ids"].([]interface{}); ok {
						runningTurns = len(ids)
					}
				}
				_ = st.updateNode(nodeID, map[string]interface{}{
					"status":        connectionStatus(st.getNode(nodeID)),
					"running_turns": runningTurns,
					"worker_count":  toInt(payload["worker_count"]),
				})
			}

			switch raw.Type {
			case "agent.state.report":
				updateNodeState()
			case "agent.ping":
				updateNodeState()
				_ = sendEnvelopeSafeForNode(agents.get(nodeID), buildEnvelope(
					"server.pong",
					"server",
					"main",
					"agent",
					nodeID,
					map[string]interface{}{"server_time": nowRFC3339()},
					raw.RequestID,
				))
			case "agent.turn.started":
				turnID := strings.TrimSpace(toString(payload["turn_id"], ""))
				if turnID != "" {
					markTurnStarted(st, turnID, nodeID)
				}
				updateNodeState()
			case "agent.turn.read":
				turnID := strings.TrimSpace(toString(payload["turn_id"], ""))
				if turnID != "" {
					markTurnRead(st, turnID, nodeID)
					if turn := st.getTurn(turnID); turn != nil {
						broadcastChatSnapshot(consoles, st, turn.ChatID)
					}
				}
				updateNodeState()
			case "agent.turn.completed":
				turnID := strings.TrimSpace(toString(payload["turn_id"], ""))
				output := strings.TrimSpace(toString(payload["output"], ""))
				if turnID != "" && output != "" {
					markTurnCompleted(st, turnID, nodeID, output)
				}
				updateNodeState()
			case "agent.workspace.validated":
				requestID := strings.TrimSpace(raw.RequestID)
				if requestID != "" {
					agents.resolveWorkspaceValidation(requestID, map[string]interface{}{
						"ok":              toBool(payload["ok"], false),
						"normalized_path": strings.TrimSpace(toString(payload["normalized_path"], "")),
						"message":         strings.TrimSpace(toString(payload["message"], "")),
					})
				}
			default:
				_ = sendEnvelopeSafeForNode(agents.get(nodeID), buildEnvelope(
					"server.notice",
					"server",
					"main",
					"agent",
					nodeID,
					map[string]interface{}{"level": "info", "message": "server ignored message type: " + raw.Type},
					raw.RequestID,
				))
			}
		}
	}
}

func connectionStatus(node *nodeRecord) string {
	if node == nil {
		return "offline"
	}
	if node.Approved {
		return "online"
	}
	return "pending"
}

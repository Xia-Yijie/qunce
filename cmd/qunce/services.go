package main

import (
	"errors"
	"fmt"
	"strings"
)

func isEmbeddedNodeRecord(node *nodeRecord, cfg appConfig) bool {
	if node == nil {
		return false
	}
	if cfg.EmbeddedNodeID != "" && strings.TrimSpace(node.NodeID) == strings.TrimSpace(cfg.EmbeddedNodeID) {
		return true
	}
	workDir := strings.TrimSpace(node.WorkDir)
	return strings.HasSuffix(workDir, `\.qunce-node`) || strings.HasSuffix(workDir, `/.qunce-node`)
}

func nodeHasBoundPersonas(nodeID string, personas []*personaRecord) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false
	}
	for _, persona := range personas {
		if persona == nil {
			continue
		}
		if strings.TrimSpace(persona.NodeID) == nodeID {
			return true
		}
	}
	return false
}

func nodeDeleteBlockReason(node *nodeRecord, cfg appConfig, personas []*personaRecord) string {
	if isEmbeddedNodeRecord(node, cfg) {
		return "伴生节点不能删除"
	}
	if nodeHasBoundPersonas(firstNonEmpty(node.NodeID), personas) {
		return "存在智能体的节点不能删除"
	}
	return ""
}

func parsePersonaIDs(raw interface{}, errorCode string) ([]string, error) {
	rawList, ok := raw.([]interface{})
	if !ok {
		if raw != nil {
			if values, ok := raw.([]string); ok {
				rawList = make([]interface{}, len(values))
				for index, value := range values {
					rawList[index] = value
				}
			}
		}
	}
	if len(rawList) == 0 {
		return nil, errors.New(errorCode)
	}

	ids := make([]string, 0, len(rawList))
	for _, item := range rawList {
		if id, ok := item.(string); ok {
			if normalized := strings.TrimSpace(id); normalized != "" {
				ids = append(ids, normalized)
			}
		}
	}
	if len(ids) == 0 {
		return nil, errors.New(errorCode)
	}
	return ids, nil
}

func loadOrderedPersonas(st *state, personaIDs []string) ([]*personaRecord, error) {
	if len(personaIDs) == 0 {
		return nil, errors.New("persona_not_found")
	}

	requested := make(map[string]struct{}, len(personaIDs))
	for _, id := range personaIDs {
		requested[id] = struct{}{}
	}

	found := make(map[string]*personaRecord)
	for _, persona := range st.listPersonas() {
		if _, ok := requested[persona.PersonaID]; ok {
			found[persona.PersonaID] = persona
		}
	}
	if len(found) != len(requested) {
		return nil, errors.New("persona_not_found")
	}

	result := make([]*personaRecord, 0, len(personaIDs))
	for _, id := range personaIDs {
		persona := found[id]
		if persona == nil {
			return nil, errors.New("persona_not_found")
		}
		result = append(result, persona)
	}
	return result, nil
}

func createUserTurn(st *state, agents *agentRegistry, chatID, content, senderName string) (map[string]interface{}, error) {
	snapshot := st.chatSnapshot(chatID)
	if snapshot == nil || snapshot.Chat == nil {
		return nil, errors.New("chat_not_found")
	}

	message := st.addMessage(&messagePayload{
		ChatID:     chatID,
		SenderType: "user",
		SenderName: senderName,
		Content:    content,
		Status:     "completed",
	})
	if message == nil {
		return nil, errors.New("chat_not_found")
	}

	personasByID := make(map[string]*personaRecord)
	for _, persona := range st.listPersonas() {
		personasByID[persona.PersonaID] = persona
	}

	snapshot = st.chatSnapshot(chatID)
	if snapshot == nil || snapshot.Chat == nil {
		return nil, errors.New("chat_not_found")
	}

	dispatched := 0
	turnCount := 0
	for _, member := range snapshot.Chat.Members {
		if member.Muted {
			continue
		}
		persona := personasByID[member.PersonaID]
		if persona == nil {
			continue
		}

		sender := firstNonEmpty(persona.Name, member.PersonaID)
		nodeID := strings.TrimSpace(persona.NodeID)
		turn := st.createTurn(
			chatID,
			content,
			nodeID,
			senderName,
			"pending",
			map[string]any{
				"message_id":    message.MessageID,
				"persona_id":    member.PersonaID,
				"persona_name":  sender,
				"workspace_dir": persona.WorkspaceDir,
				"system_prompt": persona.SystemPrompt,
				"agent_key":     persona.AgentKey,
				"agent_label":   persona.AgentLabel,
				"muted":         snapshot.Chat.Muted,
			},
		)
		if turn == nil {
			continue
		}
		turnCount++

		if nodeID == "" {
			continue
		}
		if err := dispatchTurnToAgent(st, agents, turn); err != nil {
			continue
		}
		dispatched++
	}

	return map[string]interface{}{
		"message_id":       message.MessageID,
		"turn_count":       turnCount,
		"dispatched_count": dispatched,
	}, nil
}

func dispatchTurnToAgent(st *state, agents *agentRegistry, turn *turnPayload) error {
	if st == nil || agents == nil || turn == nil {
		return errors.New("invalid turn")
	}

	nodeID := strings.TrimSpace(turn.AssignedNode)
	if nodeID == "" {
		return errors.New("node_id required")
	}

	conn := agents.get(nodeID)
	if conn == nil {
		return errors.New("agent offline")
	}

	payload := buildEnvelope(
		"server.turn.request",
		"server",
		"main",
		"agent",
		nodeID,
		map[string]any{
			"turn_id":       turn.TurnID,
			"chat_id":       turn.ChatID,
			"message_id":    turn.MessageID,
			"content":       turn.Content,
			"sender_name":   turn.SenderName,
			"persona_id":    turn.PersonaID,
			"persona_name":  firstNonEmpty(turn.PersonaName, turn.PersonaID),
			"workspace_dir": turn.WorkspaceDir,
			"system_prompt": turn.SystemPrompt,
			"agent_key":     turn.AgentKey,
			"agent_label":   turn.AgentLabel,
			"muted":         turn.Muted,
		},
		"",
	)

	if err := sendEnvelopeSafe(conn, payload); err != nil {
		agents.remove(nodeID)
		return fmt.Errorf("agent send failed: %w", err)
	}
	return nil
}

func markTurnStarted(st *state, turnID, nodeID string) *turnPayload {
	return st.updateTurn(turnID, map[string]any{
		"status":           "running",
		"assigned_node_id": nodeID,
	})
}

func markTurnRead(st *state, turnID, nodeID string) *turnPayload {
	return st.updateTurn(turnID, map[string]any{
		"status":           "read",
		"assigned_node_id": nodeID,
	})
}

func markTurnCompleted(st *state, turnID, nodeID, output string) *turnPayload {
	turn := st.updateTurn(turnID, map[string]any{
		"status":           "completed",
		"assigned_node_id": nodeID,
		"output":           output,
	})
	if turn == nil {
		return nil
	}

	senderName := firstNonEmpty(turn.PersonaName, "Agent "+nodeID)
	_ = st.addMessage(&messagePayload{
		ChatID:     turn.ChatID,
		SenderType: "agent",
		SenderName: senderName,
		Content:    firstNonEmpty(output, turn.Output, turn.Content),
		Status:     "completed",
		Metadata: map[string]interface{}{
			"turn_id":    turn.TurnID,
			"node_id":    nodeID,
			"persona_id": turn.PersonaID,
		},
	})
	return turn
}

func dispatchPendingTurnsForNode(st *state, agents *agentRegistry, nodeID string) int {
	dispatched := 0
	for _, turn := range st.listPendingTurnsForNode(nodeID) {
		if turn == nil {
			continue
		}
		if err := dispatchTurnToAgent(st, agents, turn); err != nil {
			break
		}
		dispatched++
	}
	return dispatched
}

func addChatMessage(
	st *state,
	consoles *consoleRegistry,
	chatID string,
	senderType string,
	senderName string,
	content string,
	metadata map[string]interface{},
) *chatSnapshot {
	if st == nil {
		return nil
	}
	payload := &messagePayload{
		ChatID:     chatID,
		SenderType: senderType,
		SenderName: senderName,
		Content:    content,
		Status:     "completed",
		Metadata:   metadata,
	}
	_ = st.addMessage(payload)
	if consoles != nil {
		broadcastChatSnapshot(consoles, st, chatID)
	}
	return st.chatSnapshot(chatID)
}

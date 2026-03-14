package main

import (
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
)

type routeSpec struct {
	Route   string
	Methods []string
}

var apiRouteCatalog = []routeSpec{
	{"/api/health", []string{"GET"}},
	{"/api/meta", []string{"GET"}},
	{"/api/chats", []string{"GET", "POST"}},
	{"/api/chats/<chat_id>/snapshot", []string{"GET"}},
	{"/api/chats/<chat_id>/members", []string{"POST"}},
	{"/api/chats/<chat_id>/members/<persona_id>/mute", []string{"POST"}},
	{"/api/chats/<chat_id>/members/<persona_id>", []string{"DELETE"}},
	{"/api/chats/<chat_id>", []string{"DELETE"}},
	{"/api/chats/<chat_id>/preferences", []string{"POST"}},
	{"/api/chats/<chat_id>/name", []string{"POST"}},
	{"/api/chats/<chat_id>/read", []string{"POST"}},
	{"/api/chats/<chat_id>/messages", []string{"POST", "DELETE"}},
	{"/api/chats/<chat_id>/mute", []string{"POST"}},
	{"/api/personas", []string{"GET", "POST"}},
	{"/api/nodes", []string{"GET"}},
	{"/api/nodes/<node_id>/workspace-check", []string{"POST"}},
	{"/api/nodes/<node_id>/accept", []string{"POST"}},
	{"/api/nodes/<node_id>", []string{"DELETE"}},
}

type handlerContext struct {
	state    *state
	agents   *agentRegistry
	consoles *consoleRegistry
	cfg      appConfig
}

func newHandlerContext(st *state, agents *agentRegistry, consoles *consoleRegistry, cfg appConfig) *handlerContext {
	return &handlerContext{
		state:    st,
		agents:   agents,
		consoles: consoles,
		cfg:      cfg,
	}
}

func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, message)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

func notFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found")
}

func (ctx *handlerContext) handleMeta(w http.ResponseWriter) {
	routes := make([][]any, 0, len(apiRouteCatalog))
	for _, route := range apiRouteCatalog {
		methods := append([]string(nil), route.Methods...)
		sort.Strings(methods)
		routes = append(routes, []any{route.Route, methods})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    ctx.cfg.AppName,
		"version": ctx.cfg.AppVersion,
		"routes":  routes,
	})
}

func (ctx *handlerContext) handleChats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		chats := ctx.state.listChats()
		output := make([]map[string]interface{}, 0, len(chats))
		for _, chat := range chats {
			snapshot := ctx.state.chatSnapshot(chat.ChatID)
			if snapshot != nil {
				output = append(output, chatSummaryPayload(snapshot))
				continue
			}
			output = append(output, chatSummaryPayloadFromRecord(chat))
		}
		writeJSON(w, http.StatusOK, output)
	case http.MethodPost:
		body := parseBody(r)
		rawPersonaIDs := body["persona_ids"]
		if rawPersonaIDs == nil {
			rawPersonaIDs = body["persona_id"]
		}
		if rawPersonaIDs == nil {
			badRequest(w, "persona_id_required")
			return
		}

		personaIDs, err := parsePersonaIDs(rawPersonaIDs, "persona_id_required")
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		personas, err := loadOrderedPersonas(ctx.state, personaIDs)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		name := strings.TrimSpace(toString(body["name"], ""))
		chat := ctx.state.createOrGetChat(personas, name)
		snapshot := ctx.state.chatSnapshot(chat.ChatID)
		if snapshot == nil {
			writeError(w, http.StatusNotFound, "chat_not_found")
			return
		}
		writeJSON(w, http.StatusCreated, chatSummaryPayload(snapshot))
	default:
		methodNotAllowed(w)
	}
}

func (ctx *handlerContext) handleChatSnapshot(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handleChatMembers(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	rawPersonaIDs := body["persona_ids"]
	if rawPersonaIDs == nil {
		badRequest(w, "persona_id_required")
		return
	}
	personaIDs, err := parsePersonaIDs(rawPersonaIDs, "persona_id_required")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	actorName := strings.TrimSpace(toString(body["actor_name"], "operator"))
	personas, err := loadOrderedPersonas(ctx.state, personaIDs)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	result := ctx.state.addMembersToChat(chatID, personas)
	if result == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	addedPersonaIDs := result.AddedPersonaIDs
	if len(addedPersonaIDs) == 0 {
		snapshot := ctx.state.chatSnapshot(chatID)
		if snapshot == nil {
			writeError(w, http.StatusNotFound, "chat_not_found")
			return
		}
		broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"chat":              chatSnapshotPayload(snapshot),
			"added_persona_ids": addedPersonaIDs,
		})
		return
	}

	addedNames := make([]string, 0, len(addedPersonaIDs))
	for _, personaID := range addedPersonaIDs {
		for _, persona := range personas {
			if persona.PersonaID == personaID {
				addedNames = append(addedNames, firstNonEmpty(persona.Name, personaID))
				break
			}
		}
	}
	_ = addChatMessage(
		ctx.state,
		ctx.consoles,
		chatID,
		"event",
		actorName,
		"Members added: "+strings.Join(addedNames, ", "),
		map[string]interface{}{
			"event_type":  "chat.members.added",
			"persona_ids": addedPersonaIDs,
		},
	)
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"chat":              chatSnapshotPayload(snapshot),
		"added_persona_ids": addedPersonaIDs,
	})
}

func (ctx *handlerContext) handleChatRemoveMember(w http.ResponseWriter, r *http.Request, chatID, personaID string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	actorName := strings.TrimSpace(toString(body["actor_name"], "operator"))

	result := ctx.state.removeMemberFromChat(chatID, personaID)
	if result == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if !result.Removed {
		writeError(w, http.StatusNotFound, "persona_not_in_chat")
		return
	}
	if result.Dissolved {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":                   true,
			"dissolved":            true,
			"removed_persona_id":   personaID,
			"removed_persona_name": firstNonEmpty(result.RemovedMember.Name, personaID),
		})
		return
	}

	removedName := firstNonEmpty(result.RemovedMember.Name, personaID)
	_ = addChatMessage(
		ctx.state,
		ctx.consoles,
		chatID,
		"event",
		actorName,
		"Member removed: "+removedName,
		map[string]interface{}{
			"event_type": "chat.member.removed",
			"persona_id": personaID,
		},
	)
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                   true,
		"dissolved":            false,
		"removed_persona_id":   personaID,
		"removed_persona_name": removedName,
		"chat":                 chatSnapshotPayload(snapshot),
	})
}

func (ctx *handlerContext) handleChatMuteMember(w http.ResponseWriter, r *http.Request, chatID, personaID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	muted := toBool(body["muted"], false)
	actorName := strings.TrimSpace(toString(body["actor_name"], "operator"))
	result := ctx.state.setMemberMuted(chatID, personaID, muted)
	if result == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if !result.Updated {
		writeError(w, http.StatusNotFound, "persona_not_in_chat")
		return
	}
	memberName := firstNonEmpty(result.Member.Name, personaID)
	actionText := "Member muted: " + memberName
	if !muted {
		actionText = "Member unmuted: " + memberName
	}
	_ = addChatMessage(
		ctx.state,
		ctx.consoles,
		chatID,
		"event",
		actorName,
		actionText,
		map[string]interface{}{
			"event_type": "chat.member.muted",
			"persona_id": personaID,
			"muted":      muted,
		},
	)
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handleChatDelete(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	snapshot := ctx.state.deleteChat(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"dissolved":    true,
		"chat_id":      chatID,
		"member_count": len(snapshot.Members),
	})
}

func (ctx *handlerContext) handleChatPreferences(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	nextPayload := map[string]interface{}{}
	for _, key := range []string{"pinned", "dnd", "marked_unread"} {
		if _, ok := body[key]; ok {
			nextPayload[key] = toBool(body[key], false)
		}
	}
	if len(nextPayload) == 0 {
		badRequest(w, "preferences_required")
		return
	}
	chat := ctx.state.updateChat(chatID, nextPayload)
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handleChatName(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	name := strings.TrimSpace(toString(body["name"], ""))
	if name == "" {
		badRequest(w, "chat_name_required")
		return
	}
	chat := ctx.state.updateChat(chatID, map[string]interface{}{"name": name})
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handleChatRead(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	chat := ctx.state.markChatRead(chatID)
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handleChatMessages(w http.ResponseWriter, r *http.Request, chatID string) {
	switch r.Method {
	case http.MethodDelete:
		snapshot := ctx.state.clearChatHistory(chatID)
		if snapshot == nil {
			writeError(w, http.StatusNotFound, "chat_not_found")
			return
		}
		broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
		writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
	case http.MethodPost:
		body := parseBody(r)
		content := strings.TrimSpace(toString(body["content"], ""))
		senderName := strings.TrimSpace(toString(body["sender_name"], "user"))
		if content == "" {
			badRequest(w, "message_content_required")
			return
		}
		result, err := createUserTurn(ctx.state, ctx.agents, chatID, content, senderName)
		if err != nil {
			if err.Error() == "chat_not_found" {
				writeError(w, http.StatusNotFound, "chat_not_found")
				return
			}
			writeError(w, http.StatusConflict, "current chat is busy, please retry")
			return
		}
		broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"ok":   true,
			"turn": result,
		})
	default:
		methodNotAllowed(w)
	}
}

func (ctx *handlerContext) handleChatMute(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body := parseBody(r)
	muted := toBool(body["muted"], false)
	actorName := strings.TrimSpace(toString(body["actor_name"], "operator"))
	chat := ctx.state.updateChat(chatID, map[string]interface{}{"muted": muted})
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	_ = addChatMessage(
		ctx.state,
		ctx.consoles,
		chatID,
		"event",
		actorName,
		boolText(muted, "Chat muted", "Chat unmuted"),
		map[string]interface{}{
			"event_type": "chat.muted",
			"muted":      muted,
		},
	)
	broadcastChatSnapshot(ctx.consoles, ctx.state, chatID)
	snapshot := ctx.state.chatSnapshot(chatID)
	if snapshot == nil {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	writeJSON(w, http.StatusOK, chatSnapshotPayload(snapshot))
}

func (ctx *handlerContext) handlePersonas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personas := ctx.state.listPersonas()
		output := make([]map[string]interface{}, 0, len(personas))
		for _, persona := range personas {
			output = append(output, personaSummaryPayload(persona))
		}
		writeJSON(w, http.StatusOK, output)
	case http.MethodPost:
		body := parseBody(r)
		action := strings.TrimSpace(toString(body["_action"], ""))
		if action == "delete" {
			personaID := strings.TrimSpace(toString(body["persona_id"], ""))
			if personaID == "" {
				badRequest(w, "persona_id_required")
				return
			}
			if ctx.state.deletePersona(personaID) == nil {
				writeError(w, http.StatusNotFound, "persona_not_found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
			return
		}

		if action == "update" {
			personaID := strings.TrimSpace(toString(body["persona_id"], ""))
			name := strings.TrimSpace(toString(body["name"], ""))
			avatarSymbol := strings.TrimSpace(toString(body["avatar_symbol"], ""))
			avatarBG := strings.TrimSpace(toString(body["avatar_bg_color"], ""))
			avatarText := strings.TrimSpace(toString(body["avatar_text_color"], ""))
			if personaID == "" {
				badRequest(w, "persona_id_required")
				return
			}
			if name == "" {
				badRequest(w, "persona_name_required")
				return
			}
			persona := ctx.state.updatePersona(personaID, map[string]interface{}{
				"name":              name,
				"avatar_symbol":     intLimit(firstNonEmpty(avatarSymbol, name[:1]), 1),
				"avatar_bg_color":   firstNonEmpty(avatarBG, "#d9e6f8"),
				"avatar_text_color": firstNonEmpty(avatarText, "#31547e"),
			})
			if persona == nil {
				writeError(w, http.StatusNotFound, "persona_not_found")
				return
			}
			writeJSON(w, http.StatusOK, personaSummaryPayload(persona))
			return
		}

		if action == "create_chat" {
			rawPersonaIDs := body["persona_ids"]
			if rawPersonaIDs == nil {
				badRequest(w, "persona_id_required")
				return
			}
			personaIDs, err := parsePersonaIDs(rawPersonaIDs, "persona_id_required")
			if err != nil {
				badRequest(w, err.Error())
				return
			}
			personas, err := loadOrderedPersonas(ctx.state, personaIDs)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			name := strings.TrimSpace(toString(body["name"], ""))
			chat := ctx.state.createOrGetChat(personas, name)
			snapshot := ctx.state.chatSnapshot(chat.ChatID)
			if snapshot == nil {
				writeError(w, http.StatusNotFound, "chat_not_found")
				return
			}
			writeJSON(w, http.StatusCreated, chatSummaryPayload(snapshot))
			return
		}

		name := strings.TrimSpace(toString(body["name"], ""))
		nodeID := strings.TrimSpace(toString(body["node_id"], ""))
		workspaceDir := strings.TrimSpace(toString(body["workspace_dir"], ""))
		systemPrompt := strings.TrimSpace(toString(body["system_prompt"], ""))
		agentKey := strings.TrimSpace(toString(body["agent_key"], ""))
		agentLabel := strings.TrimSpace(toString(body["agent_label"], ""))
		launchCommand := strings.TrimSpace(toString(body["launch_command"], ""))
		avatarSymbol := strings.TrimSpace(toString(body["avatar_symbol"], ""))
		avatarBG := strings.TrimSpace(toString(body["avatar_bg_color"], ""))
		avatarText := strings.TrimSpace(toString(body["avatar_text_color"], ""))

		if name == "" {
			badRequest(w, "persona_name_required")
			return
		}
		if nodeID == "" {
			badRequest(w, "node_id_required")
			return
		}
		if workspaceDir == "" {
			badRequest(w, "workspace_dir_required")
			return
		}
		if systemPrompt == "" {
			badRequest(w, "system_prompt_required")
			return
		}
		if agentKey == "" {
			badRequest(w, "agent_key_required")
			return
		}
		if launchCommand == "" {
			badRequest(w, "launch_command_required")
			return
		}

		node := ctx.state.getNode(nodeID)
		if node == nil {
			writeError(w, http.StatusNotFound, "node_not_found")
			return
		}
		if ctx.agents.get(nodeID) == nil {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"ok":      false,
				"message": "agent not connected, cannot create persona",
			})
			return
		}

		check, err := ctx.agents.requestWorkspaceValidation(nodeID, workspaceDir)
		if err != nil {
			if err.Error() == "agent workspace validation timed out" {
				writeJSON(w, http.StatusConflict, map[string]interface{}{
					"ok":      false,
					"message": "agent workspace validation timed out, please retry later",
				})
				return
			}
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"ok":      false,
				"message": "agent not connected, cannot create persona",
			})
			return
		}
		if !toBool(check["ok"], false) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"ok":              false,
				"normalized_path": strings.TrimSpace(toString(check["normalized_path"], workspaceDir)),
				"message":         strings.TrimSpace(toString(check["message"], "workspace check failed")),
			})
			return
		}

		persona := &personaRecord{
			Name:            name,
			NodeID:          nodeID,
			NodeName:        firstNonEmpty(node.Remark, node.Hostname, node.Name, nodeID),
			WorkspaceDir:    strings.TrimSpace(toString(check["normalized_path"], workspaceDir)),
			SystemPrompt:    systemPrompt,
			AgentKey:        agentKey,
			AgentLabel:      firstNonEmpty(agentLabel, agentKey),
			LaunchCommand:   launchCommand,
			ModelProvider:   firstNonEmpty(agentLabel, agentKey, "custom"),
			AvatarSymbol:    firstNonEmpty(intLimit(avatarSymbol, 1), name[:1]),
			AvatarBGColor:   firstNonEmpty(avatarBG, "#d9e6f8"),
			AvatarTextColor: firstNonEmpty(avatarText, "#31547e"),
		}
		created, err := ctx.state.addPersona(persona)
		if err != nil {
			if errors.Is(err, ErrConflict) {
				writeError(w, http.StatusConflict, "persona already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to create persona")
			return
		}
		writeJSON(w, http.StatusCreated, personaSummaryPayload(created))
	default:
		methodNotAllowed(w)
	}
}

func (ctx *handlerContext) handleNodes(w http.ResponseWriter, r *http.Request) {
	pathParts := splitPath(r.URL.Path)
	if len(pathParts) < 2 || pathParts[0] != "api" || pathParts[1] != "nodes" {
		notFound(w)
		return
	}
	if len(pathParts) == 2 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		personas := ctx.state.listPersonas()
		payload := make([]map[string]interface{}, 0, len(ctx.state.listNodes()))
		for _, node := range ctx.state.listNodes() {
			if node.Approved {
				if ctx.agents.get(node.NodeID) != nil {
					node.Status = "online"
				} else {
					node.Status = "offline"
				}
			}
			payload = append(payload, nodeSummaryPayload(node, ctx.cfg, personas))
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	nodeID := pathParts[2]
	if len(pathParts) == 3 {
		if r.Method == http.MethodDelete {
			node := ctx.state.getNode(nodeID)
			if node == nil {
				writeError(w, http.StatusNotFound, "node_not_found")
				return
			}
			if reason := nodeDeleteBlockReason(node, ctx.cfg, ctx.state.listPersonas()); reason != "" {
				writeError(w, http.StatusConflict, reason)
				return
			}
			ctx.agents.disconnect(nodeID)
			ctx.state.deleteNode(nodeID)
			broadcastNodeUpdate(ctx.consoles, ctx.state)
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
			return
		}
		methodNotAllowed(w)
		return
	}

	if len(pathParts) != 4 {
		notFound(w)
		return
	}

	switch action := pathParts[3]; action {
	case "workspace-check":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		node := ctx.state.getNode(nodeID)
		if node == nil {
			writeError(w, http.StatusNotFound, "node_not_found")
			return
		}
		body := parseBody(r)
		workspaceDir := strings.TrimSpace(toString(body["workspace_dir"], ""))
		if workspaceDir == "" {
			badRequest(w, "workspace_dir_required")
			return
		}
		if ctx.agents.get(nodeID) == nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":              false,
				"normalized_path": workspaceDir,
				"message":         "agent not connected, cannot validate",
			})
			return
		}
		result, err := ctx.agents.requestWorkspaceValidation(nodeID, workspaceDir)
		if err != nil {
			message := "validation failed"
			if err.Error() == "agent workspace validation timed out" {
				message = "agent workspace validation timed out, please retry later"
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":              false,
				"normalized_path": workspaceDir,
				"message":         message,
			})
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "accept":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		node := ctx.state.getNode(nodeID)
		if node == nil {
			writeError(w, http.StatusNotFound, "node_not_found")
			return
		}
		body := parseBody(r)
		displaySymbol := firstNonEmpty(strings.TrimSpace(toString(body["display_symbol"], "")), "N")
		remark := strings.TrimSpace(toString(body["remark"], ""))
		avatarBG := strings.TrimSpace(toString(body["avatar_bg_color"], ""))
		avatarText := strings.TrimSpace(toString(body["avatar_text_color"], ""))
		status := "offline"
		if ctx.agents.get(nodeID) != nil {
			status = "online"
		}
		accepted := ctx.state.updateNode(nodeID, map[string]interface{}{
			"approved":          true,
			"display_symbol":    displaySymbol,
			"avatar_bg_color":   firstNonEmpty(avatarBG, "#dde7df"),
			"avatar_text_color": firstNonEmpty(avatarText, "#335248"),
			"remark":            remark,
			"status":            status,
		})
		if accepted == nil {
			writeError(w, http.StatusNotFound, "node_not_found")
			return
		}
		ctx.state.refreshPersonaNodeNames(nodeID, firstNonEmpty(remark, accepted.Hostname, accepted.Name, nodeID))
		broadcastNodeUpdate(ctx.consoles, ctx.state)
		writeJSON(w, http.StatusOK, nodeSummaryPayload(accepted, ctx.cfg, ctx.state.listPersonas()))
	default:
		notFound(w)
	}
}

func splitPath(raw string) []string {
	clean := path.Clean(raw)
	parts := strings.Split(strings.Trim(clean, "/"), "/")
	if len(parts) == 1 && parts[0] == "." {
		return []string{}
	}
	return parts
}

func boolText(v bool, trueText, falseText string) string {
	if v {
		return trueText
	}
	return falseText
}

func intLimit(raw string, max int) string {
	value := strings.TrimSpace(raw)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func registerRoutes(mux *http.ServeMux, ctx *handlerContext) {
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	})
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		ctx.handleMeta(w)
	})
	mux.HandleFunc("/api/chats", ctx.handleChats)
	mux.HandleFunc("/api/personas", ctx.handlePersonas)
	mux.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		ctx.handleNodes(w, r)
	})
	mux.HandleFunc("/api/chats/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.Path)
		if len(parts) < 3 || parts[0] != "api" || parts[1] != "chats" {
			notFound(w)
			return
		}
		chatID := parts[2]
		if len(parts) == 3 {
			ctx.handleChatDelete(w, r, chatID)
			return
		}
		if len(parts) == 4 {
			switch parts[3] {
			case "snapshot":
				ctx.handleChatSnapshot(w, r, chatID)
			case "preferences":
				ctx.handleChatPreferences(w, r, chatID)
			case "name":
				ctx.handleChatName(w, r, chatID)
			case "read":
				ctx.handleChatRead(w, r, chatID)
			case "messages":
				ctx.handleChatMessages(w, r, chatID)
			case "mute":
				ctx.handleChatMute(w, r, chatID)
			case "members":
				ctx.handleChatMembers(w, r, chatID)
			default:
				notFound(w)
			}
			return
		}
		if len(parts) == 5 {
			switch parts[3] {
			case "members":
				if parts[4] == "mute" {
					notFound(w)
					return
				}
				ctx.handleChatRemoveMember(w, r, chatID, parts[4])
			default:
				notFound(w)
			}
			return
		}
		if len(parts) == 6 {
			if parts[3] == "members" && parts[5] == "mute" {
				ctx.handleChatMuteMember(w, r, chatID, parts[4])
				return
			}
		}
		notFound(w)
	})
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.Path)
		if len(parts) < 2 || parts[0] != "api" || parts[1] != "nodes" {
			notFound(w)
			return
		}
		ctx.handleNodes(w, r)
	})
}

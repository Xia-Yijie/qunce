package main

func metaPayload(appName, appVersion string) map[string]interface{} {
	return map[string]interface{}{
		"name":    appName,
		"version": appVersion,
	}
}

func chatSnapshotPayload(snapshot *chatSnapshot) map[string]interface{} {
	if snapshot == nil || snapshot.Chat == nil {
		return map[string]interface{}{}
	}

	members := make([]map[string]interface{}, 0, len(snapshot.Chat.Members))
	for _, member := range snapshot.Chat.Members {
		members = append(members, map[string]interface{}{
			"persona_id": member.PersonaID,
			"name":       member.Name,
			"status":     member.Status,
			"muted":      member.Muted,
		})
	}

	turnsByMessageID := map[string][]*turnPayload{}
	for _, turn := range snapshot.Turns {
		if turn == nil || turn.MessageID == "" {
			continue
		}
		turnsByMessageID[turn.MessageID] = append(turnsByMessageID[turn.MessageID], turn)
	}

	readReceiptMembers := func(message *messagePayload) map[string]interface{} {
		readBy := make([]map[string]interface{}, 0, len(members))
		unreadBy := make([]map[string]interface{}, 0, len(members))
		for _, member := range members {
			personaID, _ := member["persona_id"].(string)
			if personaID == "" {
				continue
			}
			if muted, _ := member["muted"].(bool); muted {
				continue
			}
			seen := false
			for _, turn := range turnsByMessageID[message.MessageID] {
				if turn.PersonaID == personaID &&
					(turn.Status == "read" || turn.Status == "running" || turn.Status == "completed") {
					seen = true
					break
				}
			}
			if seen {
				readBy = append(readBy, member)
			} else {
				unreadBy = append(unreadBy, member)
			}
		}

		return map[string]interface{}{
			"read_count":   len(readBy),
			"unread_count": len(unreadBy),
			"total_count":  len(readBy) + len(unreadBy),
			"read_by":      readBy,
			"unread_by":    unreadBy,
		}
	}

	messages := make([]map[string]interface{}, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		if message == nil || message.SenderType == "system" {
			continue
		}
		payload := map[string]interface{}{
			"message_id":  message.MessageID,
			"sender_type": message.SenderType,
			"sender_name": message.SenderName,
			"content":     message.Content,
			"status":      message.Status,
			"created_at":  message.CreatedAt,
		}
		if message.Metadata != nil {
			payload["metadata"] = message.Metadata
		} else {
			payload["metadata"] = map[string]interface{}{}
		}
		if message.SenderType == "user" {
			payload["read_receipt"] = readReceiptMembers(message)
		}
		messages = append(messages, payload)
	}

	return map[string]interface{}{
		"chat_id":       snapshot.Chat.ChatID,
		"name":          snapshot.Chat.Name,
		"mode":          snapshot.Chat.Mode,
		"muted":         snapshot.Chat.Muted,
		"pinned":        snapshot.Chat.Pinned,
		"dnd":           snapshot.Chat.Dnd,
		"marked_unread": snapshot.Chat.MarkedUnread,
		"unread_count":  snapshot.Chat.UnreadCount,
		"members":       members,
		"messages":      messages,
	}
}

func chatSummaryPayload(snapshot *chatSnapshot) map[string]interface{} {
	if snapshot == nil || snapshot.Chat == nil {
		return map[string]interface{}{}
	}

	visibleMessages := make([]*messagePayload, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		if message != nil && message.SenderType != "system" {
			visibleMessages = append(visibleMessages, message)
		}
	}
	var lastMessage *messagePayload
	if len(visibleMessages) > 0 {
		lastMessage = visibleMessages[len(visibleMessages)-1]
	}

	return map[string]interface{}{
		"chat_id":              snapshot.Chat.ChatID,
		"name":                 snapshot.Chat.Name,
		"mode":                 snapshot.Chat.Mode,
		"muted":                snapshot.Chat.Muted,
		"pinned":               snapshot.Chat.Pinned,
		"dnd":                  snapshot.Chat.Dnd,
		"marked_unread":        snapshot.Chat.MarkedUnread,
		"unread_count":         snapshot.Chat.UnreadCount,
		"member_count":         len(snapshot.Chat.Members),
		"message_count":        len(visibleMessages),
		"last_message_at":      valueOrNil(lastMessage, func(message *messagePayload) interface{} { return message.CreatedAt }),
		"last_message_preview": valueOrNil(lastMessage, func(message *messagePayload) interface{} { return message.Content }),
	}
}

func chatSummaryPayloadFromRecord(chat *chatRecord) map[string]interface{} {
	if chat == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"chat_id":              chat.ChatID,
		"name":                 chat.Name,
		"mode":                 chat.Mode,
		"muted":                chat.Muted,
		"pinned":               chat.Pinned,
		"dnd":                  chat.Dnd,
		"marked_unread":        chat.MarkedUnread,
		"unread_count":         chat.UnreadCount,
		"member_count":         len(chat.Members),
		"message_count":        0,
		"last_message_at":      nil,
		"last_message_preview": nil,
	}
}

func personaSummaryPayload(persona *personaRecord) map[string]interface{} {
	if persona == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"persona_id":        persona.PersonaID,
		"name":              persona.Name,
		"status":            persona.Status,
		"node_id":           persona.NodeID,
		"node_name":         persona.NodeName,
		"workspace_dir":     persona.WorkspaceDir,
		"system_prompt":     persona.SystemPrompt,
		"agent_key":         persona.AgentKey,
		"agent_label":       persona.AgentLabel,
		"model_provider":    persona.ModelProvider,
		"avatar_symbol":     persona.AvatarSymbol,
		"avatar_bg_color":   persona.AvatarBGColor,
		"avatar_text_color": persona.AvatarTextColor,
	}
}

func nodeSummaryPayload(node *nodeRecord) map[string]interface{} {
	if node == nil {
		return map[string]interface{}{}
	}
	statusLabel := map[string]string{
		"pending":      "pending",
		"online":       "online",
		"offline":      "offline",
		"disconnected": "disconnected",
	}[node.Status]
	if statusLabel == "" {
		statusLabel = node.Status
	}

	return map[string]interface{}{
		"node_id":        node.NodeID,
		"name":           node.Name,
		"hostname":       node.Hostname,
		"display_symbol": node.DisplaySymbol,
		"remark":         node.Remark,
		"status":         node.Status,
		"status_label":   statusLabel,
		"approved":       node.Approved,
		"can_accept":     !node.Approved,
		"hello_message":  node.HelloMessage,
		"work_dir":       node.WorkDir,
		"platform":       node.Platform,
		"arch":           node.Arch,
		"last_seen_at":   node.LastSeenAt,
		"running_turns":  node.RunningTurns,
		"worker_count":   node.WorkerCount,
	}
}

func nodeListPayload(nodes []*nodeRecord) map[string]interface{} {
	payload := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		payload = append(payload, nodeSummaryPayload(node))
	}
	return map[string]interface{}{"nodes": payload}
}

func valueOrNil[T any](value *T, selector func(value *T) interface{}) interface{} {
	if value == nil {
		return nil
	}
	return selector(value)
}

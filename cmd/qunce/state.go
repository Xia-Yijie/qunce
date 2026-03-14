package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type datastore struct {
	Chats    map[string]*chatRecord     `json:"chats"`
	Nodes    map[string]*nodeRecord     `json:"nodes"`
	Personas map[string]*personaRecord  `json:"personas"`
	Messages map[string]*messagePayload `json:"messages"`
	Turns    map[string]*turnPayload    `json:"turns"`
}

type state struct {
	mu   sync.RWMutex
	cfg  appConfig
	path string
	data *datastore
}

func newState(cfg appConfig) (*state, error) {
	path := filepath.Join(cfg.ServerDataDir, "server-state.json")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}

	data := &datastore{
		Chats:    map[string]*chatRecord{},
		Nodes:    map[string]*nodeRecord{},
		Personas: map[string]*personaRecord{},
		Messages: map[string]*messagePayload{},
		Turns:    map[string]*turnPayload{},
	}
	if raw, err := os.ReadFile(absPath); err == nil {
		if err := json.Unmarshal(raw, data); err != nil {
			return nil, fmt.Errorf("invalid state file: %w", err)
		}
		if data.Chats == nil {
			data.Chats = map[string]*chatRecord{}
		}
		if data.Nodes == nil {
			data.Nodes = map[string]*nodeRecord{}
		}
		if data.Personas == nil {
			data.Personas = map[string]*personaRecord{}
		}
		if data.Messages == nil {
			data.Messages = map[string]*messagePayload{}
		}
		if data.Turns == nil {
			data.Turns = map[string]*turnPayload{}
		}
	}

	return &state{cfg: cfg, path: absPath, data: data}, nil
}

func (s *state) persistLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

func (s *state) setNode(nodeID string, payload *nodeRecord) *nodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyNodeUpdateLocked(nodeID, payload, true)
}

func (s *state) upsertNode(nodeID string, payload map[string]interface{}) *nodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := cloneNode(s.data.Nodes[nodeID])
	if current == nil {
		current = &nodeRecord{NodeID: nodeID}
	}
	current.Name = mapString(payload["name"], current.Name)
	current.Hostname = mapString(payload["hostname"], current.Hostname)
	current.DisplaySymbol = mapString(payload["display_symbol"], current.DisplaySymbol)
	current.AvatarBGColor = mapString(payload["avatar_bg_color"], current.AvatarBGColor)
	current.AvatarTextColor = mapString(payload["avatar_text_color"], current.AvatarTextColor)
	current.Remark = mapString(payload["remark"], current.Remark)
	current.Status = mapString(payload["status"], current.Status)
	current.WorkDir = mapString(payload["work_dir"], current.WorkDir)
	current.Platform = mapString(payload["platform"], current.Platform)
	current.Arch = mapString(payload["arch"], current.Arch)
	current.HelloMessage = mapString(payload["hello_message"], current.HelloMessage)
	current.AgentVersion = mapString(payload["agent_version"], current.AgentVersion)
	if approved, ok := payload["approved"].(bool); ok {
		current.Approved = approved
	}
	if runningTurns, ok := payload["running_turns"]; ok {
		current.RunningTurns = toInt(runningTurns)
	}
	if workerCount, ok := payload["worker_count"]; ok {
		current.WorkerCount = toInt(workerCount)
	}
	current.LastSeenAt = nowRFC3339()
	if current.Status == "" {
		current.Status = "pending"
	}
	s.data.Nodes[nodeID] = current
	_ = s.persistLocked()
	return cloneNode(current)
}

func (s *state) updateNode(nodeID string, payload map[string]interface{}) *nodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.data.Nodes[nodeID]
	if current == nil {
		return nil
	}
	clone := cloneNode(current)
	clone.Name = mapString(payload["name"], clone.Name)
	clone.Hostname = mapString(payload["hostname"], clone.Hostname)
	clone.DisplaySymbol = mapString(payload["display_symbol"], clone.DisplaySymbol)
	clone.AvatarBGColor = mapString(payload["avatar_bg_color"], clone.AvatarBGColor)
	clone.AvatarTextColor = mapString(payload["avatar_text_color"], clone.AvatarTextColor)
	clone.Remark = mapString(payload["remark"], clone.Remark)
	clone.Status = mapString(payload["status"], clone.Status)
	clone.WorkDir = mapString(payload["work_dir"], clone.WorkDir)
	clone.Platform = mapString(payload["platform"], clone.Platform)
	clone.Arch = mapString(payload["arch"], clone.Arch)
	clone.HelloMessage = mapString(payload["hello_message"], clone.HelloMessage)
	clone.AgentVersion = mapString(payload["agent_version"], clone.AgentVersion)
	if approved, ok := payload["approved"].(bool); ok {
		clone.Approved = approved
	}
	if runningTurns, ok := payload["running_turns"]; ok {
		clone.RunningTurns = toInt(runningTurns)
	}
	if workerCount, ok := payload["worker_count"]; ok {
		clone.WorkerCount = toInt(workerCount)
	}
	if lastSeenAt, ok := payload["last_seen_at"].(string); ok && lastSeenAt != "" {
		clone.LastSeenAt = lastSeenAt
	} else {
		clone.LastSeenAt = nowRFC3339()
	}
	s.data.Nodes[nodeID] = clone
	_ = s.persistLocked()
	return cloneNode(clone)
}

func (s *state) applyNodeUpdateLocked(nodeID string, payload *nodeRecord, preferIncoming bool) *nodeRecord {
	current := cloneNode(s.data.Nodes[nodeID])
	if current == nil {
		current = &nodeRecord{NodeID: nodeID}
	}
	if payload == nil {
		s.data.Nodes[nodeID] = current
		_ = s.persistLocked()
		return cloneNode(current)
	}

	if preferIncoming {
		if payload.Name != "" {
			current.Name = payload.Name
		}
		if payload.Hostname != "" {
			current.Hostname = payload.Hostname
		}
		if payload.DisplaySymbol != "" {
			current.DisplaySymbol = payload.DisplaySymbol
		}
		if payload.AvatarBGColor != "" {
			current.AvatarBGColor = payload.AvatarBGColor
		}
		if payload.AvatarTextColor != "" {
			current.AvatarTextColor = payload.AvatarTextColor
		}
		if payload.Remark != "" {
			current.Remark = payload.Remark
		}
		if payload.Status != "" {
			current.Status = payload.Status
		}
		if payload.WorkDir != "" {
			current.WorkDir = payload.WorkDir
		}
		if payload.Platform != "" {
			current.Platform = payload.Platform
		}
		if payload.Arch != "" {
			current.Arch = payload.Arch
		}
		if payload.HelloMessage != "" {
			current.HelloMessage = payload.HelloMessage
		}
		if payload.AgentVersion != "" {
			current.AgentVersion = payload.AgentVersion
		}
		current.Approved = payload.Approved
		current.RunningTurns = payload.RunningTurns
		current.WorkerCount = payload.WorkerCount
	} else {
		if current.Name == "" {
			current.Name = payload.Name
		}
		if current.Hostname == "" {
			current.Hostname = payload.Hostname
		}
		if current.DisplaySymbol == "" {
			current.DisplaySymbol = payload.DisplaySymbol
		}
		if current.AvatarBGColor == "" {
			current.AvatarBGColor = payload.AvatarBGColor
		}
		if current.AvatarTextColor == "" {
			current.AvatarTextColor = payload.AvatarTextColor
		}
		if current.Remark == "" {
			current.Remark = payload.Remark
		}
		if current.Status == "" {
			current.Status = payload.Status
		}
		if current.WorkDir == "" {
			current.WorkDir = payload.WorkDir
		}
		if current.Platform == "" {
			current.Platform = payload.Platform
		}
		if current.Arch == "" {
			current.Arch = payload.Arch
		}
		if current.HelloMessage == "" {
			current.HelloMessage = payload.HelloMessage
		}
		if current.AgentVersion == "" {
			current.AgentVersion = payload.AgentVersion
		}
	}
	if current.Status == "" {
		current.Status = "pending"
	}
	current.LastSeenAt = nowRFC3339()
	s.data.Nodes[nodeID] = current
	_ = s.persistLocked()
	return cloneNode(current)
}

func mapString(raw interface{}, fallback string) string {
	value := strings.TrimSpace(toString(raw, fallback))
	if value != "" {
		return value
	}
	return fallback
}

func (s *state) getNode(nodeID string) *nodeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneNode(s.data.Nodes[nodeID])
}

func (s *state) listNodes() []*nodeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes := make([]*nodeRecord, 0, len(s.data.Nodes))
	for _, node := range s.data.Nodes {
		nodes = append(nodes, cloneNode(node))
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].LastSeenAt != nodes[j].LastSeenAt {
			return nodes[i].LastSeenAt > nodes[j].LastSeenAt
		}
		return nodes[i].NodeID > nodes[j].NodeID
	})
	return nodes
}

func (s *state) deleteNode(nodeID string) *nodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	node := cloneNode(s.data.Nodes[nodeID])
	if node == nil {
		return nil
	}
	delete(s.data.Nodes, nodeID)
	_ = s.persistLocked()
	return node
}

func (s *state) listPersonas() []*personaRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	personas := make([]*personaRecord, 0, len(s.data.Personas))
	for _, persona := range s.data.Personas {
		personas = append(personas, clonePersona(persona))
	}
	sort.Slice(personas, func(i, j int) bool {
		if personas[i].UpdatedAt != personas[j].UpdatedAt {
			return personas[i].UpdatedAt > personas[j].UpdatedAt
		}
		return personas[i].PersonaID > personas[j].PersonaID
	})
	return personas
}

func (s *state) getPersona(personaID string) *personaRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePersona(s.data.Personas[personaID])
}

func (s *state) setPersona(record *personaRecord) *personaRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.UpdatedAt = nowRFC3339()
	s.data.Personas[record.PersonaID] = clonePersona(record)
	_ = s.persistLocked()
	return clonePersona(record)
}

func (s *state) updatePersona(personaID string, payload map[string]interface{}) *personaRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	persona := s.data.Personas[personaID]
	if persona == nil {
		return nil
	}
	if name := mapString(payload["name"], persona.Name); name != "" {
		persona.Name = name
	}
	if symbol := mapString(payload["avatar_symbol"], persona.AvatarSymbol); symbol != "" {
		persona.AvatarSymbol = symbol
	}
	if bg := mapString(payload["avatar_bg_color"], persona.AvatarBGColor); bg != "" {
		persona.AvatarBGColor = bg
	}
	if text := mapString(payload["avatar_text_color"], persona.AvatarTextColor); text != "" {
		persona.AvatarTextColor = text
	}
	persona.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return clonePersona(persona)
}

func (s *state) refreshPersonaNodeNames(nodeID, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := false
	for _, persona := range s.data.Personas {
		if persona == nil || persona.NodeID != nodeID {
			continue
		}
		persona.NodeName = nodeName
		persona.UpdatedAt = nowRFC3339()
		updated = true
	}
	if updated {
		_ = s.persistLocked()
	}
}

func (s *state) addPersona(record *personaRecord) (*personaRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record == nil {
		return nil, fmt.Errorf("empty persona payload")
	}
	if record.PersonaID == "" {
		record.PersonaID = newID("persona")
	}
	if _, exists := s.data.Personas[record.PersonaID]; exists {
		return nil, NewConflictError("persona already exists")
	}
	now := nowRFC3339()
	record.Status = firstNonEmpty(record.Status, "active")
	record.ModelProvider = firstNonEmpty(record.ModelProvider, "custom")
	record.CreatedAt = firstNonEmpty(record.CreatedAt, now)
	record.UpdatedAt = now
	s.data.Personas[record.PersonaID] = clonePersona(record)
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return clonePersona(record), nil
}

func (s *state) deletePersona(personaID string) *personaRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	persona := clonePersona(s.data.Personas[personaID])
	if persona == nil {
		return nil
	}
	delete(s.data.Personas, personaID)
	_ = s.persistLocked()
	return persona
}

func (s *state) createOrGetChat(personas []*personaRecord, requestedName string) *chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createChatLocked(personas, requestedName)
}

func (s *state) createChat(personas []*personaRecord, requestedName string) *chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createChatLocked(personas, requestedName)
}

func (s *state) createChatLocked(personas []*personaRecord, requestedName string) *chatRecord {
	members := make([]chatMember, 0, len(personas))
	for _, persona := range personas {
		if persona == nil {
			continue
		}
		members = append(members, chatMember{
			PersonaID: firstNonEmpty(persona.PersonaID),
			Name:      firstNonEmpty(persona.Name),
			Status:    firstNonEmpty(persona.Status, "active"),
			Muted:     false,
		})
	}
	record := &chatRecord{
		ChatID:       newID("chat"),
		Name:         firstNonEmpty(requestedName, defaultChatName(members)),
		Mode:         "group",
		Muted:        false,
		Pinned:       false,
		Dnd:          false,
		MarkedUnread: false,
		UnreadCount:  0,
		Members:      members,
		CreatedAt:    nowRFC3339(),
		UpdatedAt:    nowRFC3339(),
	}
	s.data.Chats[record.ChatID] = record
	_ = s.persistLocked()
	return cloneChat(record)
}

func (s *state) getChat(chatID string) *chatRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneChat(s.data.Chats[chatID])
}

func (s *state) listChats() []*chatRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chats := make([]*chatRecord, 0, len(s.data.Chats))
	for _, chat := range s.data.Chats {
		chats = append(chats, cloneChat(chat))
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].Pinned != chats[j].Pinned {
			return chats[i].Pinned
		}
		if chats[i].UpdatedAt != chats[j].UpdatedAt {
			return chats[i].UpdatedAt > chats[j].UpdatedAt
		}
		return chats[i].ChatID > chats[j].ChatID
	})
	return chats
}

func (s *state) chatMessagesLocked(chatID string) []*messagePayload {
	messages := make([]*messagePayload, 0)
	for _, message := range s.data.Messages {
		if message.ChatID != chatID {
			continue
		}
		messages = append(messages, cloneMessage(message))
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt == messages[j].CreatedAt {
			return messages[i].MessageID < messages[j].MessageID
		}
		return messages[i].CreatedAt < messages[j].CreatedAt
	})
	return messages
}

func (s *state) chatTurnsLocked(chatID string) []*turnPayload {
	turns := make([]*turnPayload, 0)
	for _, turn := range s.data.Turns {
		if turn.ChatID != chatID {
			continue
		}
		turns = append(turns, cloneTurn(turn))
	}
	sort.Slice(turns, func(i, j int) bool {
		if turns[i].CreatedAt == turns[j].CreatedAt {
			return turns[i].TurnID < turns[j].TurnID
		}
		return turns[i].CreatedAt < turns[j].CreatedAt
	})
	return turns
}

func (s *state) chatSnapshot(chatID string) *chatSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chat := cloneChat(s.data.Chats[chatID])
	if chat == nil {
		return nil
	}
	messages := make([]*messagePayload, 0)
	for _, message := range s.data.Messages {
		if message.ChatID == chatID {
			messages = append(messages, cloneMessage(message))
		}
	}
	turns := make([]*turnPayload, 0)
	for _, turn := range s.data.Turns {
		if turn.ChatID == chatID {
			turns = append(turns, cloneTurn(turn))
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt == messages[j].CreatedAt {
			return messages[i].MessageID < messages[j].MessageID
		}
		return messages[i].CreatedAt < messages[j].CreatedAt
	})
	sort.Slice(turns, func(i, j int) bool {
		if turns[i].CreatedAt == turns[j].CreatedAt {
			return turns[i].TurnID < turns[j].TurnID
		}
		return turns[i].CreatedAt < turns[j].CreatedAt
	})
	return &chatSnapshot{
		Chat:     chat,
		Messages: messages,
		Turns:    turns,
	}
}

func (s *state) markChatRead(chatID string) *chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	chat.UnreadCount = 0
	chat.MarkedUnread = false
	chat.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return cloneChat(chat)
}

func (s *state) updateChat(chatID string, payload map[string]interface{}) *chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	if v, ok := payload["muted"].(bool); ok {
		chat.Muted = v
	}
	if v, ok := payload["pinned"].(bool); ok {
		chat.Pinned = v
	}
	if v, ok := payload["dnd"].(bool); ok {
		chat.Dnd = v
	}
	if v, ok := payload["marked_unread"].(bool); ok {
		chat.MarkedUnread = v
	}
	if value := mapString(payload["name"], ""); value != "" {
		chat.Name = value
	}
	chat.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return cloneChat(chat)
}

func (s *state) addMembersToChat(chatID string, personas []*personaRecord) *chatMemberMutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	existing := make(map[string]struct{}, len(chat.Members))
	for _, member := range chat.Members {
		existing[member.PersonaID] = struct{}{}
	}
	added := make([]string, 0)
	for _, persona := range personas {
		personaID := firstNonEmpty(persona.PersonaID)
		if personaID == "" {
			continue
		}
		if _, ok := existing[personaID]; ok {
			continue
		}
		chat.Members = append(chat.Members, chatMember{
			PersonaID: firstNonEmpty(persona.PersonaID),
			Name:      firstNonEmpty(persona.Name),
			Status:    firstNonEmpty(persona.Status, "active"),
			Muted:     false,
		})
		existing[personaID] = struct{}{}
		added = append(added, personaID)
	}
	if len(added) > 0 {
		chat.UpdatedAt = nowRFC3339()
	}
	_ = s.persistLocked()
	return &chatMemberMutationResult{AddedPersonaIDs: added, Chat: cloneChat(chat)}
}

func (s *state) removeMemberFromChat(chatID, personaID string) *chatMemberRemovalResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	remaining := make([]chatMember, 0, len(chat.Members))
	var removedMember *chatMember
	for _, member := range chat.Members {
		if member.PersonaID == personaID {
			copied := member
			removedMember = &copied
			continue
		}
		remaining = append(remaining, member)
	}
	if removedMember == nil {
		return &chatMemberRemovalResult{
			Chat:      cloneChat(chat),
			Removed:   false,
			Dissolved: false,
		}
	}
	if len(remaining) == 0 {
		delete(s.data.Chats, chatID)
		for messageID, message := range s.data.Messages {
			if message.ChatID == chatID {
				delete(s.data.Messages, messageID)
			}
		}
		for turnID, turn := range s.data.Turns {
			if turn.ChatID == chatID {
				delete(s.data.Turns, turnID)
			}
		}
		_ = s.persistLocked()
		return &chatMemberRemovalResult{
			Chat:          nil,
			Removed:       true,
			Dissolved:     true,
			RemovedMember: removedMember,
		}
	}
	chat.Members = remaining
	chat.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return &chatMemberRemovalResult{
		Chat:          cloneChat(chat),
		Removed:       true,
		Dissolved:     false,
		RemovedMember: removedMember,
	}
}

func (s *state) setMemberMuted(chatID, personaID string, muted bool) *chatMemberMuteResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	members := make([]chatMember, 0, len(chat.Members))
	var updatedMember *chatMember
	updated := false
	for _, member := range chat.Members {
		if member.PersonaID == personaID {
			member.Muted = muted
			copied := member
			updatedMember = &copied
			updated = true
		}
		members = append(members, member)
	}
	if !updated {
		return &chatMemberMuteResult{
			Chat:    cloneChat(chat),
			Updated: false,
		}
	}
	chat.Members = members
	chat.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return &chatMemberMuteResult{
		Chat:    cloneChat(chat),
		Updated: true,
		Member:  updatedMember,
	}
}

func (s *state) addMessage(payload *messagePayload) *messagePayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload == nil {
		return nil
	}
	if payload.MessageID == "" {
		payload.MessageID = newID("msg")
	}
	payload.Status = firstNonEmpty(payload.Status, "completed")
	payload.CreatedAt = firstNonEmpty(payload.CreatedAt, nowRFC3339())
	payload.SenderType = firstNonEmpty(payload.SenderType, "system")
	s.data.Messages[payload.MessageID] = cloneMessage(payload)

	if chat := s.data.Chats[payload.ChatID]; chat != nil {
		if payload.SenderType == "agent" {
			chat.UnreadCount++
		}
		if payload.SenderType == "user" {
			chat.UnreadCount = 0
			chat.MarkedUnread = false
		}
		chat.UpdatedAt = payload.CreatedAt
	}
	_ = s.persistLocked()
	return cloneMessage(payload)
}

func (s *state) chatMessages(chatID string) []*messagePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatMessagesLocked(chatID)
}

func (s *state) clearChatHistory(chatID string) *chatSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := s.data.Chats[chatID]
	if chat == nil {
		return nil
	}
	for id, message := range s.data.Messages {
		if message.ChatID == chatID {
			delete(s.data.Messages, id)
		}
	}
	for id, turn := range s.data.Turns {
		if turn.ChatID == chatID {
			delete(s.data.Turns, id)
		}
	}
	chat.UnreadCount = 0
	chat.MarkedUnread = false
	chat.UpdatedAt = nowRFC3339()
	_ = s.persistLocked()
	return &chatSnapshot{
		Chat:     cloneChat(chat),
		Messages: []*messagePayload{},
		Turns:    []*turnPayload{},
	}
}

func (s *state) deleteChat(chatID string) *chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat := cloneChat(s.data.Chats[chatID])
	if chat == nil {
		return nil
	}
	delete(s.data.Chats, chatID)
	for messageID, message := range s.data.Messages {
		if message.ChatID == chatID {
			delete(s.data.Messages, messageID)
		}
	}
	for turnID, turn := range s.data.Turns {
		if turn.ChatID == chatID {
			delete(s.data.Turns, turnID)
		}
	}
	_ = s.persistLocked()
	return chat
}

func (s *state) addTurn(payload *turnPayload) *turnPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload == nil || payload.ChatID == "" {
		return nil
	}
	if payload.TurnID == "" {
		payload.TurnID = newID("turn")
	}
	payload.Status = firstNonEmpty(payload.Status, "pending")
	now := nowRFC3339()
	payload.CreatedAt = firstNonEmpty(payload.CreatedAt, now)
	payload.UpdatedAt = now
	s.data.Turns[payload.TurnID] = cloneTurn(payload)
	if chat := s.data.Chats[payload.ChatID]; chat != nil {
		chat.UpdatedAt = now
	}
	_ = s.persistLocked()
	return cloneTurn(payload)
}

func (s *state) createTurn(
	chatID string,
	content string,
	nodeID string,
	senderName string,
	status string,
	extra map[string]interface{},
) *turnPayload {
	extras := map[string]any{}
	for key, value := range extra {
		extras[key] = value
	}

	turn := &turnPayload{
		ChatID:       chatID,
		Content:      content,
		Status:       firstNonEmpty(status, "pending"),
		AssignedNode: nodeID,
		SenderName:   senderName,
	}
	if senderName == "" {
		turn.SenderName = "用户"
	}

	if msgID, ok := extras["message_id"].(string); ok {
		turn.MessageID = msgID
	}
	if personaID, ok := extras["persona_id"].(string); ok {
		turn.PersonaID = personaID
	}
	if personaName, ok := extras["persona_name"].(string); ok {
		turn.PersonaName = personaName
	}
	if workspaceDir, ok := extras["workspace_dir"].(string); ok {
		turn.WorkspaceDir = workspaceDir
	}
	if systemPrompt, ok := extras["system_prompt"].(string); ok {
		turn.SystemPrompt = systemPrompt
	}
	if agentKey, ok := extras["agent_key"].(string); ok {
		turn.AgentKey = agentKey
	}
	if agentLabel, ok := extras["agent_label"].(string); ok {
		turn.AgentLabel = agentLabel
	}
	if launchCommand, ok := extras["launch_command"].(string); ok {
		turn.LaunchCommand = launchCommand
	}
	if output, ok := extras["output"].(string); ok {
		turn.Output = output
	}
	if muted, ok := extras["muted"].(bool); ok {
		turn.Muted = muted
	}

	return s.addTurn(turn)
}

func (s *state) updateTurn(turnID string, payload map[string]interface{}) *turnPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn := s.data.Turns[turnID]
	if turn == nil {
		return nil
	}
	if status, ok := payload["status"].(string); ok {
		turn.Status = status
	}
	if assigned, ok := payload["assigned_node_id"].(string); ok {
		turn.AssignedNode = assigned
	}
	if messageID, ok := payload["message_id"].(string); ok && messageID != "" {
		turn.MessageID = messageID
	}
	if output, ok := payload["output"].(string); ok && output != "" {
		turn.Output = output
	}
	turn.UpdatedAt = nowRFC3339()
	if chat := s.data.Chats[turn.ChatID]; chat != nil {
		chat.UpdatedAt = turn.UpdatedAt
	}
	_ = s.persistLocked()
	return cloneTurn(turn)
}

func (s *state) getTurn(turnID string) *turnPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneTurn(s.data.Turns[turnID])
}

func (s *state) markTurnCompleted(turnID, nodeID string) *turnPayload {
	return s.updateTurn(turnID, map[string]interface{}{
		"status":           "completed",
		"assigned_node_id": nodeID,
	})
}

func (s *state) markTurnStarted(turnID, nodeID string) *turnPayload {
	return s.updateTurn(turnID, map[string]interface{}{
		"status":           "running",
		"assigned_node_id": nodeID,
	})
}

func (s *state) markTurnRead(turnID, nodeID string) *turnPayload {
	return s.updateTurn(turnID, map[string]interface{}{
		"status":           "read",
		"assigned_node_id": nodeID,
	})
}

func (s *state) listPendingTurnsForNode(nodeID string) []*turnPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	turns := make([]*turnPayload, 0)
	for _, turn := range s.data.Turns {
		if turn.AssignedNode == nodeID && turn.Status == "pending" {
			turns = append(turns, cloneTurn(turn))
		}
	}
	sort.Slice(turns, func(i, j int) bool {
		if turns[i].CreatedAt == turns[j].CreatedAt {
			return turns[i].TurnID < turns[j].TurnID
		}
		return turns[i].CreatedAt < turns[j].CreatedAt
	})
	return turns
}

func defaultChatName(members []chatMember) string {
	names := make([]string, 0, 3)
	for _, member := range members {
		name := strings.TrimSpace(member.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
		if len(names) >= 3 {
			break
		}
	}
	if len(names) == 0 {
		return "Chat"
	}
	return strings.Join(names, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

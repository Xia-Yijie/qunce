package main

import "errors"

type chatMember struct {
	PersonaID string `json:"persona_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Muted     bool   `json:"muted"`
}

type messagePayload struct {
	MessageID  string         `json:"message_id"`
	ChatID     string         `json:"chat_id"`
	SenderType string         `json:"sender_type"`
	SenderName string         `json:"sender_name"`
	Content    string         `json:"content"`
	Status     string         `json:"status"`
	CreatedAt  string         `json:"created_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type turnPayload struct {
	TurnID       string `json:"turn_id"`
	ChatID       string `json:"chat_id"`
	MessageID    string `json:"message_id"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	AssignedNode string `json:"assigned_node_id"`
	SenderName   string `json:"sender_name"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	PersonaID    string `json:"persona_id"`
	PersonaName  string `json:"persona_name"`
	WorkspaceDir string `json:"workspace_dir"`
	SystemPrompt string `json:"system_prompt"`
	AgentKey     string `json:"agent_key"`
	AgentLabel   string `json:"agent_label"`
	Output       string `json:"output,omitempty"`
	Muted        bool   `json:"muted"`
}

type chatRecord struct {
	ChatID       string       `json:"chat_id"`
	Name         string       `json:"name"`
	Mode         string       `json:"mode"`
	Muted        bool         `json:"muted"`
	Pinned       bool         `json:"pinned"`
	Dnd          bool         `json:"dnd"`
	MarkedUnread bool         `json:"marked_unread"`
	UnreadCount  int          `json:"unread_count"`
	Members      []chatMember `json:"members"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
}

type chatSnapshot struct {
	Chat          *chatRecord       `json:"chat"`
	Messages      []*messagePayload `json:"messages"`
	Turns         []*turnPayload    `json:"turns"`
	Dissolved     bool              `json:"dissolved"`
	Removed       bool              `json:"removed"`
	RemovedMember *chatMember       `json:"removed_member,omitempty"`
}

type chatMemberMutationResult struct {
	AddedPersonaIDs []string `json:"added_persona_ids"`
	Chat            *chatRecord
}

type chatMemberRemovalResult struct {
	Chat          *chatRecord `json:"chat"`
	Removed       bool        `json:"removed"`
	Dissolved     bool        `json:"dissolved"`
	RemovedMember *chatMember `json:"removed_member"`
}

type chatMemberMuteResult struct {
	Chat    *chatRecord `json:"chat"`
	Member  *chatMember `json:"member"`
	Updated bool        `json:"updated"`
}

type nodeRecord struct {
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	DisplaySymbol string `json:"display_symbol"`
	Remark        string `json:"remark"`
	Status        string `json:"status"`
	Approved      bool   `json:"approved"`
	LastSeenAt    string `json:"last_seen_at"`
	RunningTurns  int    `json:"running_turns"`
	WorkerCount   int    `json:"worker_count"`
	WorkDir       string `json:"work_dir"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	HelloMessage  string `json:"hello_message"`
	AgentVersion  string `json:"agent_version"`
}

type personaRecord struct {
	PersonaID       string `json:"persona_id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	NodeID          string `json:"node_id"`
	NodeName        string `json:"node_name"`
	WorkspaceDir    string `json:"workspace_dir"`
	SystemPrompt    string `json:"system_prompt"`
	AgentKey        string `json:"agent_key"`
	AgentLabel      string `json:"agent_label"`
	ModelProvider   string `json:"model_provider"`
	AvatarSymbol    string `json:"avatar_symbol"`
	AvatarBGColor   string `json:"avatar_bg_color"`
	AvatarTextColor string `json:"avatar_text_color"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

var ErrConflict = errors.New("conflict")

func NewConflictError(message string) error {
	if message == "" {
		return ErrConflict
	}
	return errors.New(message)
}

func cloneChatMember(member *chatMember) *chatMember {
	if member == nil {
		return nil
	}
	next := *member
	return &next
}

func cloneMessage(message *messagePayload) *messagePayload {
	if message == nil {
		return nil
	}
	next := *message
	next.Metadata = cloneStringMap(message.Metadata)
	return &next
}

func cloneTurn(turn *turnPayload) *turnPayload {
	if turn == nil {
		return nil
	}
	next := *turn
	return &next
}

func cloneChat(chat *chatRecord) *chatRecord {
	if chat == nil {
		return nil
	}
	next := *chat
	next.Members = append([]chatMember(nil), chat.Members...)
	return &next
}

func cloneNode(node *nodeRecord) *nodeRecord {
	if node == nil {
		return nil
	}
	next := *node
	return &next
}

func clonePersona(persona *personaRecord) *personaRecord {
	if persona == nil {
		return nil
	}
	next := *persona
	return &next
}

func cloneStringMap(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(raw))
	for key, value := range raw {
		cloned[key] = value
	}
	return cloned
}

func cloneMapString(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(raw))
	for key, value := range raw {
		cloned[key] = value
	}
	return cloned
}

func cloneChatSnapshot(snapshot *chatSnapshot) *chatSnapshot {
	if snapshot == nil {
		return nil
	}

	next := &chatSnapshot{
		Chat:      cloneChat(snapshot.Chat),
		Messages:  make([]*messagePayload, 0, len(snapshot.Messages)),
		Turns:     make([]*turnPayload, 0, len(snapshot.Turns)),
		Dissolved: snapshot.Dissolved,
		Removed:   snapshot.Removed,
	}

	for _, message := range snapshot.Messages {
		next.Messages = append(next.Messages, cloneMessage(message))
	}
	for _, turn := range snapshot.Turns {
		next.Turns = append(next.Turns, cloneTurn(turn))
	}
	if snapshot.RemovedMember != nil {
		member := *snapshot.RemovedMember
		next.RemovedMember = &member
	}
	return next
}

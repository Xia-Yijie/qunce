export type ChatSummary = {
  chat_id: string;
  name: string;
  mode: string;
  muted?: boolean;
  pinned?: boolean;
  dnd?: boolean;
  marked_unread?: boolean;
  unread_count?: number;
  member_count: number;
  message_count: number;
  last_message_at?: string | null;
  last_message_preview?: string | null;
};

export type NodeSummary = {
  node_id: string;
  name: string;
  hostname: string;
  display_symbol: string;
  avatar_bg_color?: string;
  avatar_text_color?: string;
  remark: string;
  status: string;
  status_label: string;
  approved: boolean;
  can_accept: boolean;
  can_delete?: boolean;
  delete_reason?: string;
  is_embedded?: boolean;
  hello_message: string;
  work_dir?: string;
  platform?: string;
  arch?: string;
  last_seen_at?: string;
  running_turns?: number;
  worker_count?: number;
};

export type PersonaSummary = {
  persona_id: string;
  name: string;
  status: string;
  node_id: string;
  node_name: string;
  workspace_dir: string;
  system_prompt: string;
  agent_key: string;
  agent_label: string;
  launch_command: string;
  model_provider: string;
  avatar_symbol?: string;
  avatar_bg_color?: string;
  avatar_text_color?: string;
};

export type CreatePersonaPayload = {
  name: string;
  node_id: string;
  workspace_dir: string;
  system_prompt: string;
  agent_key: string;
  agent_label: string;
  launch_command: string;
  avatar_symbol: string;
  avatar_bg_color: string;
  avatar_text_color: string;
};

export type UpdatePersonaPayload = {
  persona_id: string;
  name: string;
  avatar_symbol: string;
  avatar_bg_color: string;
  avatar_text_color: string;
};

export type WorkspaceValidation = {
  ok: boolean;
  normalized_path: string;
  message: string;
};

export type ChatMember = {
  persona_id: string;
  name: string;
  status: string;
  muted?: boolean;
};

export type ChatSnapshot = {
  chat_id: string;
  name: string;
  mode: string;
  muted?: boolean;
  pinned?: boolean;
  dnd?: boolean;
  marked_unread?: boolean;
  unread_count?: number;
  members: ChatMember[];
  messages: Array<{
    message_id: string;
    sender_type: string;
    sender_name: string;
    content: string;
    status: string;
    created_at: string;
    metadata?: {
      persona_id?: string;
      node_id?: string;
      turn_id?: string;
    };
    read_receipt?: {
      read_count: number;
      unread_count: number;
      total_count: number;
      read_by: ChatMember[];
      unread_by: ChatMember[];
    };
  }>;
};

export type AddChatMembersResponse = {
  chat: ChatSnapshot;
  added_persona_ids: string[];
};

export type RemoveChatMemberResponse = {
  ok: boolean;
  dissolved: boolean;
  removed_persona_id: string;
  removed_persona_name: string;
  chat?: ChatSnapshot;
};

export type DeleteChatResponse = {
  ok: boolean;
  dissolved: boolean;
  chat_id: string;
  member_count: number;
};

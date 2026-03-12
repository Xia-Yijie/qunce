import { useEffect, useMemo, useRef, useState } from "react";
import {
  App as AntApp,
  Badge,
  Button,
  Checkbox,
  Dropdown,
  Empty,
  Flex,
  Input,
  Layout,
  List,
  Modal,
  Popover,
  Select,
  Space,
  Spin,
  Steps,
  Tooltip,
  Typography,
} from "antd";
import type { MenuProps } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";

type ChatSummary = {
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

type NodeSummary = {
  node_id: string;
  name: string;
  hostname: string;
  display_symbol: string;
  remark: string;
  status: string;
  status_label: string;
  approved: boolean;
  can_accept: boolean;
  hello_message: string;
  work_dir?: string;
  platform?: string;
  arch?: string;
  last_seen_at?: string;
  running_turns?: number;
  worker_count?: number;
};

type PersonaSummary = {
  persona_id: string;
  name: string;
  status: string;
  node_id: string;
  node_name: string;
  workspace_dir: string;
  system_prompt: string;
  agent_key: string;
  agent_label: string;
  model_provider: string;
  avatar_symbol?: string;
  avatar_bg_color?: string;
  avatar_text_color?: string;
};

type CreatePersonaPayload = {
  name: string;
  node_id: string;
  workspace_dir: string;
  system_prompt: string;
  agent_key: string;
  agent_label: string;
  avatar_symbol: string;
  avatar_bg_color: string;
  avatar_text_color: string;
};

type WorkspaceValidation = {
  ok: boolean;
  normalized_path: string;
  message: string;
};

type ChatMember = {
  persona_id: string;
  name: string;
  status: string;
  muted?: boolean;
};

type ChatSnapshot = {
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

type AddChatMembersResponse = {
  chat: ChatSnapshot;
  added_persona_ids: string[];
};

type RemoveChatMemberResponse = {
  ok: boolean;
  dissolved: boolean;
  removed_persona_id: string;
  removed_persona_name: string;
  chat?: ChatSnapshot;
};

type DeleteChatResponse = {
  ok: boolean;
  dissolved: boolean;
  chat_id: string;
  member_count: number;
};

const fetchJson = async <T,>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, init);
  if (!response.ok) {
    const text = await response.text();
    if (text) {
      let message: string | null = null;
      try {
        const payload = JSON.parse(text) as { message?: string };
        message = payload.message ?? null;
      } catch {
        message = null;
      }
      throw new Error(message || text);
    }
    throw new Error(`request failed: ${url}`);
  }
  return response.json() as Promise<T>;
};

const agentOptions = [
  { key: "codex", label: "codex" },
];

const DEFAULT_PERSONA_AVATAR_BG = "#d9e6f8";
const DEFAULT_PERSONA_AVATAR_TEXT = "#31547e";

const getPersonaAvatarConfig = (persona: Pick<PersonaSummary, "name" | "avatar_symbol" | "avatar_bg_color" | "avatar_text_color">) => ({
  symbol: (persona.avatar_symbol || persona.name || "?").trim().slice(0, 1) || "?",
  backgroundColor: persona.avatar_bg_color || DEFAULT_PERSONA_AVATAR_BG,
  color: persona.avatar_text_color || DEFAULT_PERSONA_AVATAR_TEXT,
});

const renderMutedName = (name: string, muted?: boolean) => (
  <span className="muted-name">
    {muted ? <span className="muted-name-badge">禁言</span> : null}
    <span className="muted-name-text">{name}</span>
  </span>
);

const sortChats = (chats: ChatSummary[]) =>
  [...chats].sort((left, right) => {
    if (Boolean(left.pinned) !== Boolean(right.pinned)) {
      return left.pinned ? -1 : 1;
    }
    const rightTime = right.last_message_at ?? "";
    const leftTime = left.last_message_at ?? "";
    if (rightTime !== leftTime) {
      return rightTime.localeCompare(leftTime);
    }
    return right.chat_id.localeCompare(left.chat_id);
  });

const applyChatSummaryFromSnapshot = (chat: ChatSummary, snapshot: ChatSnapshot): ChatSummary => ({
  ...chat,
  name: snapshot.name,
  muted: snapshot.muted,
  pinned: snapshot.pinned,
  dnd: snapshot.dnd,
  marked_unread: snapshot.marked_unread,
  unread_count: snapshot.unread_count ?? 0,
  member_count: snapshot.members.length,
  message_count: snapshot.messages.length,
  last_message_at:
    snapshot.messages.length > 0
      ? snapshot.messages[snapshot.messages.length - 1]?.created_at
      : null,
  last_message_preview:
    snapshot.messages.length > 0
      ? snapshot.messages[snapshot.messages.length - 1]?.content ?? null
      : null,
});

const api = {
  chats: () => fetchJson<ChatSummary[]>("/api/chats"),
  createChat: (payload: { personaIds: string[]; name?: string }) =>
    fetchJson<ChatSummary>("/api/chats", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        persona_ids: payload.personaIds,
        name: payload.name,
      }),
    }),
  nodes: () => fetchJson<NodeSummary[]>("/api/nodes"),
  acceptNode: (nodeId: string, payload: { display_symbol: string; remark: string }) =>
    fetchJson<NodeSummary>(`/api/nodes/${nodeId}/accept`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  rejectNode: (nodeId: string) => fetchJson<{ ok: boolean }>(`/api/nodes/${nodeId}`, { method: "DELETE" }),
  chatSnapshot: (chatId: string) => fetchJson<ChatSnapshot>(`/api/chats/${chatId}/snapshot`),
  addChatMembers: (chatId: string, payload: { personaIds: string[]; actorName?: string }) =>
    fetchJson<AddChatMembersResponse>(`/api/chats/${chatId}/members`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        persona_ids: payload.personaIds,
        actor_name: payload.actorName,
      }),
    }),
  removeChatMember: (chatId: string, personaId: string, payload?: { actorName?: string }) =>
    fetchJson<RemoveChatMemberResponse>(`/api/chats/${chatId}/members/${personaId}`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ actor_name: payload?.actorName }),
    }),
  deleteChat: (chatId: string) =>
    fetchJson<DeleteChatResponse>(`/api/chats/${chatId}`, {
      method: "DELETE",
    }),
  toggleChatMute: (chatId: string, muted: boolean) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/mute`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ muted, actor_name: "群状态" }),
    }),
  toggleChatMemberMute: (chatId: string, personaId: string, muted: boolean) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/members/${personaId}/mute`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ muted, actor_name: "群成员" }),
    }),
  updateChatPreferences: (chatId: string, payload: { pinned?: boolean; dnd?: boolean; markedUnread?: boolean }) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/preferences`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...(payload.pinned === undefined ? {} : { pinned: payload.pinned }),
        ...(payload.dnd === undefined ? {} : { dnd: payload.dnd }),
        ...(payload.markedUnread === undefined ? {} : { marked_unread: payload.markedUnread }),
      }),
    }),
  clearChatHistory: (chatId: string) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/messages`, {
      method: "DELETE",
    }),
  markChatRead: (chatId: string) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/read`, {
      method: "POST",
    }),
  updateChatName: (chatId: string, name: string) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/name`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }),
  personas: () => fetchJson<PersonaSummary[]>("/api/personas"),
  createPersona: (payload: CreatePersonaPayload) =>
    fetchJson<PersonaSummary>("/api/personas", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  deletePersona: (personaId: string) =>
    fetchJson<{ ok: boolean }>("/api/personas", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ _action: "delete", persona_id: personaId }),
    }),
  validateWorkspace: (nodeId: string, workspaceDir: string) =>
    fetchJson<WorkspaceValidation>(`/api/nodes/${nodeId}/workspace-check`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ workspace_dir: workspaceDir }),
    }),
  sendMessage: (chatId: string, content: string) =>
    fetchJson<{ ok: boolean }>(`/api/chats/${chatId}/messages`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content, sender_name: "你" }),
    }),
};

const ChatRailIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="rail-icon">
    <path
      d="M5 6.5C5 5.12 6.12 4 7.5 4H16.5C17.88 4 19 5.12 19 6.5V12.5C19 13.88 17.88 15 16.5 15H11L7 19V15H7.5C6.12 15 5 13.88 5 12.5V6.5Z"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <path d="M8.5 8.5H15.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    <path d="M8.5 11.5H13.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
  </svg>
);

const AgentRailIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="rail-icon">
    <circle cx="9" cy="9" r="2.5" fill="none" stroke="currentColor" strokeWidth="1.8" />
    <circle cx="15.5" cy="10" r="2.2" fill="none" stroke="currentColor" strokeWidth="1.8" />
    <path
      d="M5.5 18C5.8 15.7 7.65 14 10 14H10.6C12.95 14 14.8 15.7 15.1 18"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
    />
    <path
      d="M14 17.5C14.25 16.05 15.45 15 16.95 15H17.1C18.55 15 19.75 16 20 17.4"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
    />
  </svg>
);

const StartChatMenuIcon = () => (
  <svg viewBox="0 0 20 20" aria-hidden="true" className="quick-create-menu-icon">
    <path
      d="M4 5.5C4 4.67 4.67 4 5.5 4H14.5C15.33 4 16 4.67 16 5.5V10.5C16 11.33 15.33 12 14.5 12H9L6 15V12H5.5C4.67 12 4 11.33 4 10.5V5.5Z"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

const AddFriendMenuIcon = () => (
  <svg viewBox="0 0 20 20" aria-hidden="true" className="quick-create-menu-icon">
    <circle cx="8" cy="7" r="2.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
    <path d="M3.8 15C4.2 12.9 5.9 11.5 8 11.5C10.1 11.5 11.8 12.9 12.2 15" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    <path d="M15 6V10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    <path d="M13 8H17" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

const SettingsRailIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="rail-icon">
    <path
      d="M12 8.8A3.2 3.2 0 1 0 12 15.2A3.2 3.2 0 1 0 12 8.8Z"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
    />
    <path
      d="M19 13.1V10.9L16.9 10.3C16.7 9.7 16.45 9.15 16.1 8.65L17.2 6.7L15.3 4.8L13.35 5.9C12.85 5.55 12.3 5.3 11.7 5.1L11.1 3H8.9L8.3 5.1C7.7 5.3 7.15 5.55 6.65 5.9L4.7 4.8L2.8 6.7L3.9 8.65C3.55 9.15 3.3 9.7 3.1 10.3L1 10.9V13.1L3.1 13.7C3.3 14.3 3.55 14.85 3.9 15.35L2.8 17.3L4.7 19.2L6.65 18.1C7.15 18.45 7.7 18.7 8.3 18.9L8.9 21H11.1L11.7 18.9C12.3 18.7 12.85 18.45 13.35 18.1L15.3 19.2L17.2 17.3L16.1 15.35C16.45 14.85 16.7 14.3 16.9 13.7L19 13.1Z"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinejoin="round"
    />
  </svg>
);

const MuteChatIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="composer-mute-icon">
    <path
      d="M5 14H8.5L13 18V6L8.5 10H5V14Z"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <path d="M16.5 9L20 15" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    <path d="M20 9L16.5 15" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
  </svg>
);

const MoreActionsIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="chat-more-icon">
    <circle cx="6" cy="12" r="1.8" fill="currentColor" />
    <circle cx="12" cy="12" r="1.8" fill="currentColor" />
    <circle cx="18" cy="12" r="1.8" fill="currentColor" />
  </svg>
);

const AddMemberIcon = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="chat-detail-add-icon">
    <path d="M12 6V18" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    <path d="M6 12H18" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
  </svg>
);

const railItems = [
  { key: "/chats", icon: <ChatRailIcon />, description: "聊天" },
  { key: "/friends", icon: <AgentRailIcon />, description: "Agent" },
];

const settingsRailItem = { key: "/settings/runtime", icon: <SettingsRailIcon />, description: "设置" };

const getPendingNodeCount = (nodes: NodeSummary[]) => nodes.filter((node) => node.can_accept).length;

const getChatPath = (chatId: string) => `/chats/${chatId}`;

const StartChatDialog = ({
  open,
  personas,
  onClose,
  onCreated,
}: {
  open: boolean;
  personas: PersonaSummary[];
  onClose: () => void;
  onCreated: (chat: ChatSummary) => void;
}) => {
  const { message } = AntApp.useApp();
  const [chatName, setChatName] = useState("");
  const [keyword, setKeyword] = useState("");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (!open) {
      setChatName("");
      setKeyword("");
      setSelectedIds([]);
      setCreating(false);
    }
  }, [open]);

  const filteredPersonas = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    if (!query) {
      return personas;
    }
    return personas.filter((persona) => {
      const haystack = `${persona.name} ${persona.system_prompt} ${persona.node_name}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [keyword, personas]);

  const togglePersona = (personaId: string) => {
    setSelectedIds((current) =>
      current.includes(personaId) ? current.filter((item) => item !== personaId) : [...current, personaId],
    );
  };

  const submit = async () => {
    if (selectedIds.length === 0 || creating) {
      return;
    }
    setCreating(true);
    try {
      const chat = await api.createChat({ personaIds: selectedIds, name: chatName.trim() || undefined });
      onCreated(chat);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "发起聊天失败");
    } finally {
      setCreating(false);
    }
  };

  return (
    <Modal
      open={open}
      onCancel={() => !creating && onClose()}
      footer={null}
      centered
      width={720}
      title="发起群聊"
      destroyOnHidden
    >
      <div className="start-chat-dialog">
        <Input
          value={chatName}
          onChange={(event) => setChatName(event.target.value)}
          placeholder="群聊名称，不填则默认使用成员名"
          maxLength={40}
        />
        <Input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder="搜索智能体"
          className="start-chat-search"
        />
        <div className="start-chat-dialog-body">
          <div className="start-chat-list">
            {filteredPersonas.length === 0 ? (
              <div className="start-chat-empty">暂无可选的智能体，请先创建角色后再发起聊天。</div>
            ) : (
              filteredPersonas.map((persona) => {
                const checked = selectedIds.includes(persona.persona_id);
                return (
                  <button
                    key={persona.persona_id}
                    type="button"
                    className={`start-chat-persona ${checked ? "selected" : ""}`}
                    onClick={() => togglePersona(persona.persona_id)}
                  >
                    <Checkbox checked={checked} />
                    <PersonaAvatar persona={persona} className="directory-avatar agent" />
                    <div className="directory-copy">
                      <Typography.Text strong>{persona.name}</Typography.Text>
                      <Typography.Text type="secondary">{persona.system_prompt || "未设置角色设定"}</Typography.Text>
                    </div>
                  </button>
                );
              })
            )}
          </div>
          <div className="start-chat-selection">
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              已选择的智能体
            </Typography.Title>
            {selectedIds.length === 0 ? (
               <div className="start-chat-empty">请选择至少一个智能体加入会话，右侧会显示当前已选成员。</div>
            ) : (
              <div className="start-chat-selected-tags">
                {selectedIds.map((personaId) => {
                  const persona = personas.find((item) => item.persona_id === personaId);
                  if (!persona) {
                    return null;
                  }
                  return (
                    <button key={personaId} type="button" className="start-chat-tag" onClick={() => togglePersona(personaId)}>
                      {persona.name}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
        <div className="start-chat-actions">
          <Button onClick={onClose} disabled={creating}>
            取消
          </Button>
          <Button type="primary" onClick={() => void submit()} loading={creating} disabled={selectedIds.length === 0}>
            创建聊天
          </Button>
        </div>
      </div>
    </Modal>
  );
};

const AddMembersDialog = ({
  open,
  chatId,
  personas,
  existingPersonaIds,
  onClose,
  onAdded,
}: {
  open: boolean;
  chatId: string | null;
  personas: PersonaSummary[];
  existingPersonaIds: string[];
  onClose: () => void;
  onAdded: (payload: AddChatMembersResponse) => void;
}) => {
  const { message } = AntApp.useApp();
  const [keyword, setKeyword] = useState("");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) {
      setKeyword("");
      setSelectedIds([]);
      setSaving(false);
    }
  }, [open]);

  const availablePersonas = useMemo(
    () => personas.filter((persona) => !existingPersonaIds.includes(persona.persona_id)),
    [existingPersonaIds, personas],
  );

  const filteredPersonas = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    if (!query) {
      return availablePersonas;
    }
    return availablePersonas.filter((persona) => {
      const haystack = `${persona.name} ${persona.system_prompt} ${persona.node_name}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [availablePersonas, keyword]);

  const togglePersona = (personaId: string) => {
    setSelectedIds((current) =>
      current.includes(personaId) ? current.filter((item) => item !== personaId) : [...current, personaId],
    );
  };

  const submit = async () => {
    if (!chatId || selectedIds.length === 0 || saving) {
      return;
    }
    setSaving(true);
    try {
      const payload = await api.addChatMembers(chatId, { personaIds: selectedIds, actorName: "群成员" });
      onAdded(payload);
      if (payload.added_persona_ids.length === 0) {
        message.info("所选成员已在当前聊天中");
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : "添加成员失败");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open={open} onCancel={() => !saving && onClose()} footer={null} centered width={720} title="添加成员" destroyOnHidden>
      <div className="start-chat-dialog">
        <Input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜索成员" className="start-chat-search" />
        <div className="start-chat-dialog-body">
          <div className="start-chat-list">
            {filteredPersonas.length === 0 ? (
              <div className="start-chat-empty">没有可添加的成员</div>
            ) : (
              filteredPersonas.map((persona) => {
                const checked = selectedIds.includes(persona.persona_id);
                return (
                  <button
                    key={persona.persona_id}
                    type="button"
                    className={`start-chat-persona ${checked ? "selected" : ""}`}
                    onClick={() => togglePersona(persona.persona_id)}
                  >
                    <Checkbox checked={checked} />
                    <PersonaAvatar persona={persona} className="directory-avatar agent" />
                    <div className="directory-copy">
                      <Typography.Text strong>{persona.name}</Typography.Text>
                      <Typography.Text type="secondary">{persona.system_prompt || "未设置角色设定"}</Typography.Text>
                    </div>
                  </button>
                );
              })
            )}
          </div>
          <div className="start-chat-selection">
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              已选成员
            </Typography.Title>
            {selectedIds.length === 0 ? (
              <div className="start-chat-empty">选择要加入当前聊天的成员</div>
            ) : (
              <div className="start-chat-selected-tags">
                {selectedIds.map((personaId) => {
                  const persona = personas.find((item) => item.persona_id === personaId);
                  if (!persona) {
                    return null;
                  }
                  return (
                    <button key={personaId} type="button" className="start-chat-tag" onClick={() => togglePersona(personaId)}>
                      {persona.name}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
        <div className="start-chat-actions">
          <Button onClick={onClose} disabled={saving}>
            取消
          </Button>
          <Button type="primary" onClick={() => void submit()} loading={saving} disabled={selectedIds.length === 0 || !chatId}>
            添加
          </Button>
        </div>
      </div>
    </Modal>
  );
};
const useConsoleSocket = (chatIds: string[]) => {
  const queryClient = useQueryClient();
  const [connected, setConnected] = useState(false);
  const [lastNotice, setLastNotice] = useState("等待连接群聊通道");

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/console`);

    socket.addEventListener("open", () => {
      setConnected(true);
      setLastNotice("群聊通道已连接");
      socket.send(
        JSON.stringify({
          v: 1,
          type: "console.subscribe",
          event_id: `evt_${crypto.randomUUID()}`,
          request_id: `req_${crypto.randomUUID()}`,
          ts: new Date().toISOString(),
          source: { kind: "console", id: "browser" },
          target: { kind: "server", id: "main" },
          data: { chat_ids: chatIds, watch_nodes: true },
        }),
      );
    });

    socket.addEventListener("message", (event) => {
      const payload = JSON.parse(event.data) as { type: string; data?: unknown };
      if (payload.type === "server.notice") {
        const data = (payload.data ?? {}) as Record<string, unknown>;
        setLastNotice(String(data.message ?? "收到服务器通知"));
      }
      if (payload.type === "server.node.updated") {
        setLastNotice("节点状态已同步");
        const data = (payload.data ?? {}) as { nodes?: NodeSummary[] };
        queryClient.setQueryData(["nodes"], data.nodes ?? []);
      }
      if (payload.type === "server.chat.snapshot") {
        const data = payload.data as ChatSnapshot;
        queryClient.setQueryData(["chat", data.chat_id], data);
        queryClient.setQueryData(["chats"], (previous: ChatSummary[] | undefined) =>
          sortChats(
            (previous ?? []).map((chat) =>
              chat.chat_id === data.chat_id ? applyChatSummaryFromSnapshot(chat, data) : chat,
            ),
          ),
        );
      }
    });

    socket.addEventListener("close", () => {
      setConnected(false);
      setLastNotice("群聊通道已断开");
    });

    socket.addEventListener("error", () => {
      setConnected(false);
      setLastNotice("群聊通道连接失败");
    });

    return () => socket.close();
  }, [chatIds.join("|"), queryClient]);

  return { connected, lastNotice };
};

const getChatPreview = (chat: ChatSummary) => {
  if (chat.last_message_preview?.trim()) {
    return chat.last_message_preview.trim();
  }
  return "还没有消息，等待第一轮讨论";
};

const formatMessageTime = (raw: string) => {
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
};

const formatConversationTime = (raw?: string | null) => {
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const now = new Date();
  const isSameDay =
    now.getFullYear() === date.getFullYear() &&
    now.getMonth() === date.getMonth() &&
    now.getDate() === date.getDate();

  if (isSameDay) {
    return new Intl.DateTimeFormat("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    }).format(date);
  }

  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
  }).format(date);
};

const formatReadableTime = (raw?: string) => {
  if (!raw) {
    return "-";
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
};

const getNodeName = (node: Pick<NodeSummary, "name" | "hostname">) => {
  const hostname = node.hostname.trim();
  if (hostname) {
    return hostname;
  }
  const fallback = node.name.split("@").pop()?.trim();
  return fallback || node.name;
};

const joinWorkspacePath = (baseDir: string, name: string) => {
  const trimmedBase = baseDir.trim().replace(/[\\/]+$/, "");
  const sanitizedName = name.trim().replace(/[<>:"/\\|?*]+/g, "_") || "friend";
  if (!trimmedBase) {
    return sanitizedName;
  }
  const separator = trimmedBase.includes("\\") ? "\\" : "/";
  return `${trimmedBase}${separator}${sanitizedName}`;
};

const getNodeDisplayName = (node: Pick<NodeSummary, "remark" | "name" | "hostname">) => {
  const remark = node.remark.trim();
  if (remark) {
    return remark;
  }
  return getNodeName(node);
};

const PersonaAvatar = ({
  persona,
  className,
}: {
  persona: Pick<PersonaSummary, "name" | "avatar_symbol" | "avatar_bg_color" | "avatar_text_color">;
  className: string;
}) => {
  const avatar = getPersonaAvatarConfig(persona);
  return (
    <div className={className} style={{ background: avatar.backgroundColor, color: avatar.color }}>
      {avatar.symbol}
    </div>
  );
};

const PersonaInfoCard = ({ persona }: { persona: PersonaSummary }) => (
  <div className="agent-card-popover">
    <div className="agent-card-head">
      <PersonaAvatar persona={persona} className="agent-card-avatar" />
      <div className="agent-card-copy">
        <Typography.Text strong>{persona.name}</Typography.Text>
      </div>
    </div>
    <div className="agent-card-grid">
      <div className="agent-card-row">
        <span className="agent-card-label">运行节点</span>
        <span>{persona.node_name || persona.node_id || "-"}</span>
      </div>
      <div className="agent-card-row">
        <span className="agent-card-label">工作目录</span>
        <span className="agent-card-value">{persona.workspace_dir || "-"}</span>
      </div>
      <div className="agent-card-row">
        <span className="agent-card-label">角色设定</span>
        <span className="agent-card-value">{persona.system_prompt || "-"}</span>
      </div>
    </div>
  </div>
);

const PersonaInteractiveAvatar = ({
  persona,
  member,
  avatarClassName,
  buttonClassName,
  popoverPlacement,
  onMention,
  onMute,
  onKick,
}: {
  persona: PersonaSummary;
  member?: ChatMember | null;
  avatarClassName: string;
  buttonClassName: string;
  popoverPlacement: "rightTop" | "leftTop";
  onMention?: (persona: PersonaSummary) => void;
  onMute?: (persona: PersonaSummary, member?: ChatMember | null) => void;
  onKick?: (persona: PersonaSummary) => void;
}) => {
  const menuItems: MenuProps["items"] = [
    { key: "mention", label: `@ ${persona.name}` },
    { key: "mute", label: member?.muted ? "取消禁言" : "禁言" },
    { key: "kick", label: "踢出群聊", danger: true },
  ];

  return (
    <Dropdown
      trigger={["contextMenu"]}
      menu={{
        items: menuItems,
        onClick: ({ key, domEvent }) => {
          domEvent.preventDefault();
          if (key === "mention") {
            onMention?.(persona);
            return;
          }
          if (key === "mute") {
            onMute?.(persona, member);
            return;
          }
          if (key === "kick") {
            onKick?.(persona);
          }
        },
      }}
    >
      <span className="message-avatar-trigger">
        <Popover placement={popoverPlacement} content={<PersonaInfoCard persona={persona} />} trigger="click" overlayClassName="agent-card-popover-wrap">
          <button type="button" className={buttonClassName} aria-label={`查看 ${persona.name} 的智能体信息`}>
            <PersonaAvatar persona={persona} className={avatarClassName} />
          </button>
        </Popover>
      </span>
    </Dropdown>
  );
};

const getMessageTone = (senderType: string) => {
  if (senderType === "event") {
    return "event";
  }
  if (senderType === "system") {
    return "system";
  }
  if (senderType === "user") {
    return "user";
  }
  return "agent";
};

const QuickCreateMenu = () => {
  const navigate = useNavigate();

  return (
    <Dropdown
      overlayClassName="quick-create-dropdown"
      trigger={["click"]}
      menu={{
        items: [
          {
            key: "start-chat",
            label: (
              <span className="quick-create-menu-label">
                <StartChatMenuIcon />
                <span>发起群聊</span>
              </span>
            ),
          },
          {
            key: "add-friend",
            label: (
              <span className="quick-create-menu-label">
                <AddFriendMenuIcon />
                <span>添加伙伴</span>
              </span>
            ),
          },
        ],
        onClick: ({ key, domEvent }) => {
          domEvent.stopPropagation();
          if (key === "start-chat") {
            navigate("/chats?create=chat");
            return;
          }
          if (key === "add-friend") {
            navigate("/friends?create=friend");
          }
        },
      }}
    >
      <button type="button" className="conversation-plus" aria-label="快捷操作">
        +
      </button>
    </Dropdown>
  );
};

const ConversationList = ({
  chats,
  selectedChatId,
  onTogglePin,
  onToggleDnd,
  onToggleUnread,
  onClearHistory,
  onDissolveChat,
}: {
  chats: ChatSummary[];
  selectedChatId: string;
  onTogglePin: (chat: ChatSummary) => void;
  onToggleDnd: (chat: ChatSummary) => void;
  onToggleUnread: (chat: ChatSummary) => void;
  onClearHistory: (chat: ChatSummary) => void;
  onDissolveChat: (chat: ChatSummary) => void;
}) => (
  <aside className="conversation-pane">
    <div className="conversation-header">
      <div className="conversation-search-row">
        <div className="conversation-search">搜索</div>
        <QuickCreateMenu />
      </div>
    </div>
    <List
      className="conversation-list"
      dataSource={chats}
      renderItem={(chat) => {
        const isActive = chat.chat_id === selectedChatId;
        const menuItems: MenuProps["items"] = [
          { key: "pin", label: chat.pinned ? "取消置顶" : "置顶" },
          { key: "dnd", label: chat.dnd ? "取消免打扰" : "免打扰" },
          { key: "unread", label: chat.marked_unread ? "取消标为未读" : "标为未读" },
          { type: "divider" },
          { key: "clear", label: "清除聊天记录" },
          { key: "dissolve", label: "解散群聊", danger: true },
        ];
        return (
          <Dropdown
            trigger={["contextMenu"]}
            menu={{
              items: menuItems,
              onClick: ({ key, domEvent }) => {
                domEvent.preventDefault();
                domEvent.stopPropagation();
                if (key === "pin") {
                  onTogglePin(chat);
                  return;
                }
                if (key === "dnd") {
                  onToggleDnd(chat);
                  return;
                }
                if (key === "unread") {
                  onToggleUnread(chat);
                  return;
                }
                if (key === "clear") {
                  onClearHistory(chat);
                  return;
                }
                if (key === "dissolve") {
                  onDissolveChat(chat);
                }
              },
            }}
          >
            <List.Item className={`conversation-row ${isActive ? "active" : ""}`}>
              <Link to={getChatPath(chat.chat_id)} className="conversation-link">
                <Badge
                  count={
                    chat.dnd
                      ? 0
                      : (chat.unread_count ?? 0) > 0
                        ? chat.unread_count
                        : chat.marked_unread
                          ? 1
                          : undefined
                  }
                  size="default"
                  offset={[-2, 34]}
                >
                  <div className="conversation-avatar">{chat.name.slice(0, 1)}</div>
                </Badge>
                <div className="conversation-copy">
                  <Flex justify="space-between" align="center" gap={12}>
                    <Flex align="center" gap={8} className="conversation-title-row">
                      {chat.pinned ? <span className="conversation-flag">置顶</span> : null}
                      {chat.dnd ? <span className="conversation-flag quiet">免打扰</span> : null}
                      <Typography.Text strong>{chat.name}</Typography.Text>
                    </Flex>
                    <Typography.Text type="secondary">{formatConversationTime(chat.last_message_at)}</Typography.Text>
                  </Flex>
                  <Typography.Text type="secondary" className="conversation-preview">
                    {getChatPreview(chat)}
                  </Typography.Text>
                </div>
              </Link>
            </List.Item>
          </Dropdown>
        );
      }}
    />
  </aside>
);

const DirectoryLayout = ({
  title,
  sections,
  selectedId,
  onSelect,
  renderRow,
  detail,
}: {
  title: string;
  sections: Array<{ key: string; title: string; defaultCollapsed?: boolean; items: Array<{ id: string }> }>;
  selectedId: string | null;
  onSelect: (id: string) => void;
  renderRow: (id: string, active: boolean) => React.ReactNode;
  detail: React.ReactNode;
}) => {
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(sections.map((section) => [section.key, section.defaultCollapsed ?? false])),
  );

  useEffect(() => {
    setCollapsedSections((current) => {
      const next = { ...current };
      let changed = false;
      for (const section of sections) {
        if (!(section.key in next)) {
          next[section.key] = section.defaultCollapsed ?? false;
          changed = true;
        }
      }
      return changed ? next : current;
    });
  }, [sections]);

  return (
    <section className="directory-shell">
      <aside className="directory-list-pane">
        <div className="directory-toolbar">
          <div className="conversation-search-row">
            <div className="conversation-search">搜索</div>
            <QuickCreateMenu />
          </div>
        </div>
        <div className="directory-sections">
          {sections.map((section) => {
            const collapsed = collapsedSections[section.key] ?? false;
            return (
              <div key={section.key} className="directory-section">
                <button
                  type="button"
                  className="directory-section-header"
                  onClick={() =>
                    setCollapsedSections((current) => ({
                      ...current,
                      [section.key]: !(current[section.key] ?? false),
                    }))
                  }
                >
                  <span className={`directory-section-caret ${collapsed ? "collapsed" : ""}`}>▾</span>
                  <Typography.Text className="directory-section-title">{section.title}</Typography.Text>
                </button>
                {!collapsed ? (
                  section.items.length === 0 ? (
                    <div className="directory-empty">暂无</div>
                  ) : (
                    <List
                      className="directory-list"
                      dataSource={section.items}
                      renderItem={(item) => (
                        <List.Item className={`directory-row ${selectedId === item.id ? "active" : ""}`}>
                          <div
                            className="directory-row-button"
                            role="button"
                            tabIndex={0}
                            onClick={() => onSelect(item.id)}
                            onKeyDown={(event) => {
                              if (event.key === "Enter" || event.key === " ") {
                                event.preventDefault();
                                onSelect(item.id);
                              }
                            }}
                          >
                            {renderRow(item.id, selectedId === item.id)}
                          </div>
                        </List.Item>
                      )}
                    />
                  )
                ) : null}
              </div>
            );
          })}
        </div>
      </aside>
      <section className="directory-detail-pane">{detail}</section>
    </section>
  );
};

const MessageBubble = ({
  senderName,
  senderType,
  content,
  createdAt,
  persona,
  member,
  onMention,
  onMuteAgent,
  onKickAgent,
  readReceipt,
}: {
  senderName: string;
  senderType: string;
  content: string;
  createdAt: string;
  persona?: PersonaSummary | null;
  member?: ChatMember | null;
  onMention?: (persona: PersonaSummary) => void;
  onMuteAgent?: (persona: PersonaSummary, member?: ChatMember | null) => void;
  onKickAgent?: (persona: PersonaSummary) => void;
  readReceipt?: {
    read_count: number;
    unread_count: number;
    total_count: number;
    read_by: ChatMember[];
    unread_by: ChatMember[];
  };
}) => {
  const tone = getMessageTone(senderType);
  if (tone === "event") {
    return (
      <div className="message-row event">
        <div className="message-event-badge">
          <Typography.Text type="secondary">{content}</Typography.Text>
        </div>
      </div>
    );
  }
  const showReadReceipt = senderType === "user" && readReceipt && readReceipt.total_count > 0;
  const readReceiptPanel = readReceipt ? (
    <div className="message-read-panel">
      <div className="message-read-column">
        <Typography.Text strong>{readReceipt.read_count} 已读</Typography.Text>
        {readReceipt.read_by.length > 0 ? (
          readReceipt.read_by.map((member) => (
            <div key={`read-${member.persona_id}`} className="message-read-member">
              {renderMutedName(member.name, member.muted)}
            </div>
          ))
        ) : (
          <div className="message-read-empty">暂无</div>
        )}
      </div>
      <div className="message-read-column">
        <Typography.Text strong>{readReceipt.unread_count} 未读</Typography.Text>
        {readReceipt.unread_by.length > 0 ? (
          readReceipt.unread_by.map((member) => (
            <div key={`unread-${member.persona_id}`} className="message-read-member">
              {renderMutedName(member.name, member.muted)}
            </div>
          ))
        ) : (
          <div className="message-read-empty">暂无</div>
        )}
      </div>
    </div>
  ) : null;
  const avatar =
    senderType === "agent" && persona ? (
      <PersonaAvatar persona={persona} className={`message-avatar ${tone}`} />
    ) : (
      <div className={`message-avatar ${tone}`}>{senderName.slice(0, 1)}</div>
    );
  return (
    <div className={`message-row ${tone}`}>
      {senderType === "agent" && persona ? (
        <PersonaInteractiveAvatar
          persona={persona}
          member={member}
          avatarClassName={`message-avatar ${tone}`}
          buttonClassName="message-avatar-button"
          popoverPlacement="rightTop"
          onMention={onMention}
          onMute={onMuteAgent}
          onKick={onKickAgent}
        />
      ) : (
        avatar
      )}
      <div className="message-body">
        <Flex align="center" gap={10}>
          <Typography.Text strong>{renderMutedName(senderName, member?.muted)}</Typography.Text>
          <Typography.Text type="secondary" className="message-time">
            {formatMessageTime(createdAt)}
          </Typography.Text>
        </Flex>
        <div className={`message-bubble-wrap ${showReadReceipt ? "with-read-receipt" : ""}`}>
          <div className={`message-bubble ${tone}`}>{content}</div>
          {showReadReceipt ? (
            <div className="message-read-anchor">
              <Popover placement="leftBottom" content={readReceiptPanel} trigger="click" overlayClassName="message-read-popover">
                <button
                  type="button"
                  className="message-read-badge"
                  aria-label="查看已读状态"
                >
                  <span className="message-read-count">
                    {readReceipt.read_count}/{readReceipt.total_count}
                  </span>
                </button>
              </Popover>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
};

const ChatPage = ({ connected, lastNotice, chatId: forcedChatId }: { connected: boolean; lastNotice: string; chatId?: string }) => {
  const { chatId } = useParams();
  const navigate = useNavigate();
  const activeChatId = forcedChatId ?? chatId;
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [pendingMuted, setPendingMuted] = useState<boolean | null>(null);
  const [muting, setMuting] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [memberKeyword, setMemberKeyword] = useState("");
  const [addMembersOpen, setAddMembersOpen] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const messageStreamRef = useRef<HTMLDivElement | null>(null);
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const { data: chats = [] } = useQuery({ queryKey: ["chats"], queryFn: api.chats });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });
  const { data: snapshot, isLoading } = useQuery({
    queryKey: ["chat", activeChatId],
    queryFn: () => api.chatSnapshot(activeChatId ?? ""),
    enabled: Boolean(activeChatId),
  });

  useEffect(() => {
    const container = messageStreamRef.current;
    if (!container) {
      return;
    }
    container.scrollTop = container.scrollHeight;
  }, [activeChatId, snapshot?.messages.length]);

  useEffect(() => {
    if (!activeChatId || !snapshot) {
      return;
    }
    if ((snapshot.unread_count ?? 0) === 0 && !snapshot.marked_unread) {
      return;
    }
    void api.markChatRead(activeChatId).then((nextSnapshot) => {
      syncChatSnapshot(nextSnapshot);
    });
  }, [activeChatId, snapshot?.messages.length, snapshot?.unread_count, snapshot?.marked_unread]);

  useEffect(() => {
    setPendingMuted(null);
    setMuting(false);
    setDetailOpen(false);
    setMemberKeyword("");
    setAddMembersOpen(false);
    setRenaming(false);
  }, [activeChatId]);

  const syncChatSnapshot = (nextSnapshot: ChatSnapshot) => {
    queryClient.setQueryData(["chat", nextSnapshot.chat_id], nextSnapshot);
    queryClient.setQueryData(["chats"], (previous: ChatSummary[] | undefined) =>
      sortChats(
        (previous ?? []).map((chat) =>
          chat.chat_id === nextSnapshot.chat_id ? applyChatSummaryFromSnapshot(chat, nextSnapshot) : chat,
        ),
      ),
    );
  };

  const sendMessage = async () => {
    const content = draft.trim();
    if (!content || sending || !activeChatId) {
      return;
    }
    setSending(true);
    try {
      await api.sendMessage(activeChatId, content);
      setDraft("");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "发送消息失败");
    } finally {
      setSending(false);
    }
  };

  const toggleMute = async () => {
    if (!activeChatId || !snapshot) {
      return;
    }
    const nextMuted = !effectiveMuted;
    setPendingMuted(nextMuted);
    setMuting(true);
    try {
      const nextSnapshot = await api.toggleChatMute(activeChatId, nextMuted);
      syncChatSnapshot(nextSnapshot);
      message.success(nextMuted ? "已开启全体禁言" : "已关闭全体禁言");
    } catch (error) {
      setPendingMuted((current) => (current === nextMuted ? null : current));
      message.error(error instanceof Error ? error.message : "更新群聊状态失败");
    } finally {
      setMuting(false);
    }
  };

  const effectiveMuted = pendingMuted ?? Boolean(snapshot?.muted);

  if (isLoading) {
    return (
      <div className="wechat-shell loading-shell">
        <Spin />
      </div>
    );
  }

  if (!activeChatId) {
    return (
      <div className="wechat-shell">
        <ConversationList
          chats={chats}
          selectedChatId=""
          onTogglePin={togglePinChat}
          onToggleDnd={toggleDndChat}
          onToggleUnread={toggleUnreadChat}
          onClearHistory={clearConversationHistory}
          onDissolveChat={dissolveConversation}
        />
        <div className="chat-pane empty-pane">
          <div className="empty-state-panel">
            <Empty description="还没有聊天室" />
          </div>
        </div>
      </div>
    );
  }

  if (!snapshot) {
    return (
      <div className="wechat-shell">
        <ConversationList
          chats={chats}
          selectedChatId={activeChatId}
          onTogglePin={togglePinChat}
          onToggleDnd={toggleDndChat}
          onToggleUnread={toggleUnreadChat}
          onClearHistory={clearConversationHistory}
          onDissolveChat={dissolveConversation}
        />
        <div className="chat-pane empty-pane">
          <div className="empty-state-panel">
          <Empty description="房间不存在" />
          </div>
        </div>
      </div>
    );
  }

  const filteredMembers = snapshot.members.filter((member) => {
    const query = memberKeyword.trim().toLowerCase();
    if (!query) {
      return true;
    }
    return `${member.name} ${member.persona_id}`.toLowerCase().includes(query);
  });
  const detailMetaItems = [
    { label: "成员数量", value: `${snapshot.members.length} 人` },
    { label: "全体禁言", value: effectiveMuted ? "已开启" : "未开启" },
  ];
  const membersById = new Map(snapshot.members.map((member) => [member.persona_id, member]));
  const personasById = new Map(personas.map((persona) => [persona.persona_id, persona]));
  const personasByName = new Map(personas.map((persona) => [persona.name, persona]));
  const mentionPersona = (persona: PersonaSummary) => {
    setDraft((current) => {
      const prefix = current.trim().length === 0 ? "" : current.endsWith(" ") ? current : `${current} `;
      return `${prefix}@${persona.name} `;
    });
    window.requestAnimationFrame(() => {
      document.getElementById("chat-composer")?.focus();
    });
  };
  const mutePersona = async (persona: PersonaSummary, member?: ChatMember | null) => {
    if (!activeChatId) {
      return;
    }
    const targetMember = member ?? membersById.get(persona.persona_id);
    if (!targetMember) {
      message.error(`当前聊天中找不到成员 ${persona.name}`);
      return;
    }
    const nextMuted = !Boolean(targetMember.muted);
    try {
      const nextSnapshot = await api.toggleChatMemberMute(activeChatId, persona.persona_id, nextMuted);
      syncChatSnapshot(nextSnapshot);
      message.success(nextMuted ? `已禁言 ${persona.name}` : `已取消禁言 ${persona.name}`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新成员禁言状态失败");
    }
  };
  const handleMembersAdded = (payload: AddChatMembersResponse) => {
    syncChatSnapshot(payload.chat);
    setAddMembersOpen(false);
    if (payload.added_persona_ids.length > 0) {
      message.success(`已添加 ${payload.added_persona_ids.length} 个成员`);
    } else {
      message.info("所选成员已在当前聊天中");
    }
  };
  const applyDissolvedChat = (chatIdToRemove: string) => {
    queryClient.removeQueries({ queryKey: ["chat", chatIdToRemove] });
    queryClient.setQueryData(
      ["chats"],
      (previous: ChatSummary[] | undefined) => (previous ?? []).filter((chat) => chat.chat_id !== chatIdToRemove),
    );
    setAddMembersOpen(false);
    navigate("/chats");
  };
  const kickPersona = async (persona: PersonaSummary) => {
    if (!activeChatId) {
      return;
    }
    Modal.confirm({
      title: "踢出群聊",
      content: `确认将 ${persona.name} 踢出当前聊天吗？`,
      okText: "踢出",
      okButtonProps: { danger: true },
      cancelText: "取消",
      centered: true,
      onOk: async () => {
        const payload = await api.removeChatMember(activeChatId, persona.persona_id, { actorName: "群成员" });
        if (payload.dissolved) {
          applyDissolvedChat(activeChatId);
          message.success("最后一个成员已移出，群聊已解散");
          return;
        }
        if (!payload.chat) {
          return;
        }
        syncChatSnapshot(payload.chat);
        message.success(`已移出 ${persona.name}`);
      },
    });
  };
  const dissolveChat = async () => {
    if (!activeChatId) {
      return;
    }
    Modal.confirm({
      title: "解散群聊",
      content: "确认解散当前群聊吗？解散后将无法恢复。",
      okText: "解散",
      okButtonProps: { danger: true },
      cancelText: "取消",
      centered: true,
      onOk: async () => {
        await api.deleteChat(activeChatId);
        applyDissolvedChat(activeChatId);
        message.success("群聊已解散");
      },
    });
  };
  const renameChat = () => {
    if (!activeChatId || renaming) {
      return;
    }
    let draftName = snapshot.name;
    Modal.confirm({
      title: "编辑聊天名称",
      centered: true,
      okText: "保存",
      cancelText: "取消",
      content: (
        <Input
          autoFocus
          defaultValue={snapshot.name}
          maxLength={40}
          placeholder="输入聊天名称"
          onChange={(event) => {
            draftName = event.target.value;
          }}
        />
      ),
      onOk: async () => {
        const nextName = draftName.trim();
        if (!nextName) {
          message.error("聊天名称不能为空");
          throw new Error("chat_name_required");
        }
        setRenaming(true);
        try {
          const nextSnapshot = await api.updateChatName(activeChatId, nextName);
          syncChatSnapshot(nextSnapshot);
          message.success("已更新聊天名称");
        } finally {
          setRenaming(false);
        }
      },
    });
  };
  async function updateConversationPreference(
    chat: ChatSummary,
    payload: { pinned?: boolean; dnd?: boolean; markedUnread?: boolean },
    successText: string,
  ) {
    try {
      const nextSnapshot = await api.updateChatPreferences(chat.chat_id, payload);
      syncChatSnapshot(nextSnapshot);
      message.success(successText);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "更新会话状态失败");
    }
  }
  function togglePinChat(chat: ChatSummary) {
    void updateConversationPreference(chat, { pinned: !chat.pinned }, chat.pinned ? "已取消置顶" : "已置顶");
  }
  function toggleDndChat(chat: ChatSummary) {
    void updateConversationPreference(chat, { dnd: !chat.dnd }, chat.dnd ? "已关闭免打扰" : "已开启免打扰");
  }
  function toggleUnreadChat(chat: ChatSummary) {
    void updateConversationPreference(
      chat,
      { markedUnread: !chat.marked_unread },
      chat.marked_unread ? "已取消标为未读" : "已标为未读",
    );
  }
  function clearConversationHistory(chat: ChatSummary) {
    Modal.confirm({
      title: "清除聊天记录",
      content: `确认清除 ${chat.name} 的聊天记录吗？清除后无法恢复。`,
      okText: "清除",
      okButtonProps: { danger: true },
      cancelText: "取消",
      centered: true,
      onOk: async () => {
        const nextSnapshot = await api.clearChatHistory(chat.chat_id);
        syncChatSnapshot(nextSnapshot);
        message.success("已清除聊天记录");
      },
    });
  }
  function dissolveConversation(chat: ChatSummary) {
    Modal.confirm({
      title: "解散群聊",
      content: `确认解散 ${chat.name} 吗？解散后将无法恢复。`,
      okText: "解散",
      okButtonProps: { danger: true },
      cancelText: "取消",
      centered: true,
      onOk: async () => {
        await api.deleteChat(chat.chat_id);
        if (chat.chat_id === activeChatId) {
          applyDissolvedChat(chat.chat_id);
        } else {
          queryClient.setQueryData(
            ["chats"],
            (previous: ChatSummary[] | undefined) => (previous ?? []).filter((item) => item.chat_id !== chat.chat_id),
          );
        }
        message.success("群聊已解散");
      },
    });
  }

  return (
    <div className={`wechat-shell ${detailOpen ? "detail-open" : ""}`}>
      <ConversationList
        chats={chats}
        selectedChatId={activeChatId}
        onTogglePin={togglePinChat}
        onToggleDnd={toggleDndChat}
        onToggleUnread={toggleUnreadChat}
        onClearHistory={clearConversationHistory}
        onDissolveChat={dissolveConversation}
      />

      <div className="chat-stage">
        <section className="chat-pane">
          <header className="chat-header">
            <div className="chat-header-copy">
              <Typography.Title level={4} style={{ margin: 0 }}>
                {snapshot.name}
              </Typography.Title>
              <Button type="link" size="small" className="chat-rename-button" onClick={renameChat} loading={renaming}>
                编辑
              </Button>
            </div>
            <button
              type="button"
              className={`chat-more-button ${detailOpen ? "active" : ""}`}
              aria-label={detailOpen ? "收起详情" : "查看详情"}
              aria-pressed={detailOpen}
              onClick={() => setDetailOpen((current) => !current)}
            >
              <MoreActionsIcon />
            </button>
          </header>

          <div ref={messageStreamRef} className="message-stream">
            {snapshot.messages.map((message) => (
              <MessageBubble
                key={message.message_id}
                senderName={message.sender_name}
                senderType={message.sender_type}
                content={message.content}
                createdAt={message.created_at}
                persona={
                  (message.metadata?.persona_id ? personasById.get(message.metadata.persona_id) : undefined) ??
                  personasByName.get(message.sender_name) ??
                  null
                }
                member={message.metadata?.persona_id ? membersById.get(message.metadata.persona_id) ?? null : null}
                onMention={mentionPersona}
                onMuteAgent={mutePersona}
                onKickAgent={kickPersona}
                readReceipt={message.read_receipt}
              />
            ))}
          </div>

          <footer className="composer-panel">
            <div className="composer-toolbar">
              <Tooltip title="全体禁言">
                <Button
                  className={`composer-mute ${effectiveMuted ? "active" : ""}`}
                  type="text"
                  aria-label="全体禁言"
                  aria-pressed={effectiveMuted}
                  disabled={muting}
                  onClick={() => void toggleMute()}
                >
                  <MuteChatIcon />
                </Button>
              </Tooltip>
            </div>
            <Input.TextArea
              id="chat-composer"
              autoSize={{ minRows: 4, maxRows: 7 }}
              bordered={false}
              placeholder={effectiveMuted ? "输入消息，当前仅你自己可发言" : "输入消息，后续这里会接真实发送能力"}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onPressEnter={(event) => {
                if (!event.shiftKey) {
                  event.preventDefault();
                  void sendMessage();
                }
              }}
            />
            <Flex justify="flex-end" align="center">
              <Button className="send-button" type="default" onClick={() => void sendMessage()} loading={sending}>
                发送
              </Button>
            </Flex>
          </footer>
        </section>

        {detailOpen ? (
          <aside className="chat-detail-pane">
            <div className="chat-detail-search">
              <Input
                value={memberKeyword}
                onChange={(event) => setMemberKeyword(event.target.value)}
                placeholder="搜索成员"
                allowClear
              />
            </div>
            <div className="chat-detail-members">
              {filteredMembers.map((member) => (
                <div key={member.persona_id} className="chat-detail-member">
                  {personasById.get(member.persona_id) ? (
                    <PersonaInteractiveAvatar
                      persona={personasById.get(member.persona_id)!}
                      member={member}
                      avatarClassName="chat-detail-avatar"
                      buttonClassName="chat-detail-avatar-button"
                      popoverPlacement="leftTop"
                      onMention={mentionPersona}
                      onMute={mutePersona}
                      onKick={kickPersona}
                    />
                  ) : (
                    <div className="chat-detail-avatar">{member.name.slice(0, 1)}</div>
                  )}
                  <div className="chat-detail-name">{renderMutedName(member.name, member.muted)}</div>
                </div>
              ))}
              <button
                type="button"
                className="chat-detail-member chat-detail-member-add"
                onClick={() => setAddMembersOpen(true)}
              >
                <div className="chat-detail-avatar chat-detail-avatar-add">
                  <AddMemberIcon />
                </div>
                <div className="chat-detail-name">添加</div>
              </button>
            </div>
            <div className="chat-detail-sections">
              <section className="chat-detail-section">
                <div className="chat-detail-label">聊天名称</div>
                <div className="chat-detail-value chat-detail-name-row">
                  <span>{snapshot.name}</span>
                  <Button type="link" size="small" onClick={renameChat} loading={renaming}>
                    编辑
                  </Button>
                </div>
              </section>
              {detailMetaItems.map((item) => (
                <section key={item.label} className="chat-detail-section">
                  <div className="chat-detail-label">{item.label}</div>
                  <div className="chat-detail-value">{item.value}</div>
                </section>
              ))}
            </div>
            <div className="chat-detail-actions">
              <Button danger block onClick={() => void dissolveChat()}>
                解散群聊
              </Button>
            </div>
          </aside>
        ) : null}
        <AddMembersDialog
          open={addMembersOpen}
          chatId={activeChatId ?? null}
          personas={personas}
          existingPersonaIds={snapshot.members.map((member) => member.persona_id)}
          onClose={() => setAddMembersOpen(false)}
          onAdded={handleMembersAdded}
        />
      </div>
    </div>
  );
};

const SetupPage = () => {
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });

  return (
    <section className="panel-page">
      <div className="panel-card">
        <Typography.Title level={3}>开始配置</Typography.Title>
        <Typography.Paragraph>
          在这里完成节点、工作目录和智能体的基础配置。配置完成后即可进入群聊主界面，开始第一轮协作讨论。
        </Typography.Paragraph>
        <List
          dataSource={[
            `当前在线节点：${nodes.filter((node) => node.status === "online").length}`,
            `已配置 Agent：${personas.length}`,
            "下一步：进入群聊主界面发起第一轮讨论",
          ]}
          renderItem={(item) => <List.Item>{item}</List.Item>}
        />
      </div>
    </section>
  );
};

const FriendWizard = ({
  open,
  nodes,
  onClose,
  onCreated,
}: {
  open: boolean;
  nodes: NodeSummary[];
  onClose: () => void;
  onCreated: (persona: PersonaSummary) => void;
}) => {
  const { message } = AntApp.useApp();
  const approvedNodes = useMemo(() => nodes.filter((node) => node.approved), [nodes]);
  const [step, setStep] = useState(0);
  const [saving, setSaving] = useState(false);
  const [validatingWorkspace, setValidatingWorkspace] = useState(false);
  const [workspaceNotice, setWorkspaceNotice] = useState<{ ok: boolean; text: string } | null>(null);
  const [draft, setDraft] = useState<CreatePersonaPayload>({
    name: "",
    node_id: "",
    workspace_dir: "",
    system_prompt: "",
    agent_key: agentOptions[0].key,
    agent_label: agentOptions[0].label,
    avatar_symbol: "",
    avatar_bg_color: DEFAULT_PERSONA_AVATAR_BG,
    avatar_text_color: DEFAULT_PERSONA_AVATAR_TEXT,
  });

  useEffect(() => {
    if (!open) {
      setStep(0);
      setSaving(false);
      setValidatingWorkspace(false);
      setWorkspaceNotice(null);
      setDraft({
        name: "",
        node_id: "",
        workspace_dir: "",
        system_prompt: "",
        agent_key: agentOptions[0].key,
        agent_label: agentOptions[0].label,
        avatar_symbol: "",
        avatar_bg_color: DEFAULT_PERSONA_AVATAR_BG,
        avatar_text_color: DEFAULT_PERSONA_AVATAR_TEXT,
      });
    }
  }, [open]);

  useEffect(() => {
    if (!draft.node_id && approvedNodes[0]) {
      setDraft((current) => ({ ...current, node_id: approvedNodes[0].node_id }));
    }
  }, [approvedNodes, draft.node_id]);

  const selectedAgent = agentOptions.find((item) => item.key === draft.agent_key) ?? agentOptions[0];
  const selectedNode = approvedNodes.find((node) => node.node_id === draft.node_id) ?? null;
  const canContinue =
    (step === 0 && Boolean(draft.name.trim()) && Boolean(draft.node_id)) ||
    (step === 1 && Boolean(draft.workspace_dir.trim())) ||
    (step === 2 && Boolean(draft.system_prompt.trim())) ||
    step === 3;

  const submit = async () => {
    setSaving(true);
    try {
      const persona = await api.createPersona({
        ...draft,
        name: draft.name.trim(),
        workspace_dir: draft.workspace_dir.trim(),
        system_prompt: draft.system_prompt.trim(),
        agent_label: selectedAgent.label,
        avatar_symbol: (draft.avatar_symbol || draft.name || "?").trim().slice(0, 1),
      });
      message.success(`已创建智能体 ${persona.name}`);
      onCreated(persona);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建智能体失败");
    } finally {
      setSaving(false);
    }
  };

  const goNext = async () => {
    if (step !== 1) {
      setStep((current) => current + 1);
      return;
    }

    const workspaceDir = draft.workspace_dir.trim();
    if (!draft.node_id || !workspaceDir) {
      return;
    }

    setValidatingWorkspace(true);
    setWorkspaceNotice(null);
    try {
      const result = await api.validateWorkspace(draft.node_id, workspaceDir);
      setWorkspaceNotice({ ok: result.ok, text: result.message });
      if (!result.ok) {
        return;
      }
      if (result.normalized_path && result.normalized_path !== draft.workspace_dir) {
        setDraft((current) => ({ ...current, workspace_dir: result.normalized_path }));
      }
      setStep((current) => current + 1);
    } catch (error) {
      setWorkspaceNotice({
        ok: false,
        text: error instanceof Error ? error.message : "创建失败",
      });
    } finally {
      setValidatingWorkspace(false);
    }
  };

  return (
    <Modal open={open} onCancel={() => !saving && onClose()} footer={null} centered width={640} title="添加朋友" destroyOnHidden>
      <div className="friend-wizard">
        <Steps current={step} size="small" items={[{ title: "基本设定" }, { title: "工作目录" }, { title: "角色设定" }, { title: "底层工具" }]} />
        <div className="friend-wizard-panel">
          {step === 0 ? (
            <div className="wizard-field-stack">
              <Input
                placeholder="输入智能体名称，例如：代码助手"
                value={draft.name}
                onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
              />
              <Select
                placeholder="选择要部署该智能体的节点"
                value={draft.node_id || undefined}
                onChange={(value) => {
                  setWorkspaceNotice(null);
                  setDraft((current) => ({ ...current, node_id: value }));
                }}
                options={approvedNodes.map((node) => ({
                  value: node.node_id,
                  label: `${getNodeDisplayName(node)} · ${node.status_label}`,
                }))}
                notFoundContent="暂无可选节点"
              />
              <div className="wizard-avatar-config">
                <div className="wizard-avatar-preview-card">
                  <PersonaAvatar
                    persona={{
                      name: draft.name || "智",
                      avatar_symbol: draft.avatar_symbol,
                      avatar_bg_color: draft.avatar_bg_color,
                      avatar_text_color: draft.avatar_text_color,
                    }}
                    className="wizard-avatar-preview"
                  />
                  <Typography.Text type="secondary">头像预览</Typography.Text>
                </div>
                <div className="wizard-avatar-fields">
                  <Input
                    maxLength={1}
                    value={draft.avatar_symbol}
                    placeholder="头像单字"
                    onChange={(event) => setDraft((current) => ({ ...current, avatar_symbol: event.target.value.slice(0, 1) }))}
                  />
                  <label className="wizard-color-field">
                    <span>底色</span>
                    <input
                      type="color"
                      value={draft.avatar_bg_color}
                      onChange={(event) => setDraft((current) => ({ ...current, avatar_bg_color: event.target.value }))}
                    />
                  </label>
                  <label className="wizard-color-field">
                    <span>字色</span>
                    <input
                      type="color"
                      value={draft.avatar_text_color}
                      onChange={(event) => setDraft((current) => ({ ...current, avatar_text_color: event.target.value }))}
                    />
                  </label>
                </div>
              </div>
            </div>
          ) : null}
          {step === 1 ? (
            <div className="wizard-field-stack">
              <Input
                placeholder="工作目录，例如：C:\\Users\\NINGMEI\\Projects\\demo"
                value={draft.workspace_dir}
                onChange={(event) => {
                  setWorkspaceNotice(null);
                  setDraft((current) => ({ ...current, workspace_dir: event.target.value }));
                }}
              />
              {selectedNode?.work_dir ? (
                <div className="wizard-path-actions">
                  <Button
                    onClick={() => {
                      setWorkspaceNotice(null);
                      setDraft((current) => ({
                        ...current,
                        workspace_dir: joinWorkspacePath(selectedNode.work_dir ?? "", draft.name),
                      }));
                    }}
                  >
                    使用节点默认目录
                  </Button>
                </div>
              ) : null}
              {selectedNode?.work_dir ? (
                <Typography.Text type="secondary" className="wizard-path-hint">
                  {`工作目录将创建在节点目录 ${selectedNode.work_dir}`}
                </Typography.Text>
              ) : null}
              <Typography.Text type="secondary" className="wizard-path-hint">
                工作目录用于存放该智能体的仓库或任务文件，建议选择节点可持久访问的位置。
              </Typography.Text>
              {workspaceNotice ? (
                <div className={`wizard-path-notice ${workspaceNotice.ok ? "ok" : "error"}`}>{workspaceNotice.text}</div>
              ) : null}
            </div>
          ) : null}
          {step === 2 ? (
            <div className="wizard-field-stack">
              <Input.TextArea
                className="wizard-textarea-fill"
                autoSize={{ minRows: 10, maxRows: 16 }}
                placeholder="输入系统提示词，描述这个智能体的职责、风格、边界和输出要求"
                value={draft.system_prompt}
                onChange={(event) => setDraft((current) => ({ ...current, system_prompt: event.target.value }))}
              />
            </div>
          ) : null}
          {step === 3 ? (
            <div className="wizard-field-stack">
              <Select
                value={draft.agent_key}
                onChange={(value) => {
                  const selected = agentOptions.find((item) => item.key === value) ?? agentOptions[0];
                  setDraft((current) => ({ ...current, agent_key: selected.key, agent_label: selected.label }));
                }}
                options={agentOptions.map((item) => ({ value: item.key, label: item.label }))}
              />
            </div>
          ) : null}
        </div>
        <div className="friend-wizard-footer">
          <Button onClick={() => (step === 0 ? onClose() : setStep((current) => current - 1))} disabled={saving}>
            {step === 0 ? "取消" : "上一步"}
          </Button>
          {step < 3 ? (
            <Button type="primary" disabled={!canContinue || saving || validatingWorkspace} onClick={() => void goNext()}>
              下一步
            </Button>
          ) : (
            <Button type="primary" loading={saving} onClick={() => void submit()} disabled={approvedNodes.length === 0}>
              完成
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
};

type DirectoryEntry =
  | ({ kind: "agent"; id: string } & PersonaSummary)
  | ({ kind: "node"; id: string } & NodeSummary);

const AgentDirectoryPage = ({ defaultKind }: { defaultKind: "agent" | "node" }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });
  const [pendingNodeId, setPendingNodeId] = useState<string | null>(null);
  const [accepting, setAccepting] = useState(false);
  const [displaySymbolDraft, setDisplaySymbolDraft] = useState("");
  const [remarkDraft, setRemarkDraft] = useState("");
  const entries = useMemo<DirectoryEntry[]>(
    () => [
      ...personas.map((persona) => ({ ...persona, kind: "agent" as const, id: `agent:${persona.persona_id}` })),
      ...nodes.map((node) => ({ ...node, kind: "node" as const, id: `node:${node.node_id}` })),
    ],
    [nodes, personas],
  );
  const [selectedEntryId, setSelectedEntryId] = useState<string | null>(null);
  const selectedEntry = useMemo(() => {
    if (selectedEntryId) {
      return entries.find((entry) => entry.id === selectedEntryId) ?? null;
    }
    return entries.find((entry) => entry.kind === defaultKind) ?? entries[0] ?? null;
  }, [defaultKind, entries, selectedEntryId]);

  useEffect(() => {
    if (!selectedEntry && entries.length > 0) {
      const first = entries.find((entry) => entry.kind === defaultKind) ?? entries[0];
      setSelectedEntryId(first.id);
    }
  }, [defaultKind, entries, selectedEntry]);

  const sections = useMemo(
    () => [
      {
        key: "nodes",
        title: "节点",
        items: nodes.map((node) => ({ id: `node:${node.node_id}` })),
      },
      {
        key: "agents",
        title: "智能体",
        items: personas.map((persona) => ({ id: `agent:${persona.persona_id}` })),
      },
    ],
    [nodes, personas],
  );

  const pendingNode = useMemo(
    () => nodes.find((node) => node.node_id === pendingNodeId) ?? null,
    [nodes, pendingNodeId],
  );
  const openCreate = searchParams.get("create") === "friend";

  const openAcceptDialog = (node: NodeSummary) => {
    setPendingNodeId(node.node_id);
    setDisplaySymbolDraft(node.display_symbol || getNodeDisplayName(node).slice(0, 1));
    setRemarkDraft(node.remark || "");
  };

  const closeAcceptDialog = () => {
    if (accepting) {
      return;
    }
    setPendingNodeId(null);
    setDisplaySymbolDraft("");
    setRemarkDraft("");
  };

  const acceptNode = async (nodeId: string) => {
    setAccepting(true);
    const acceptedNode = await api.acceptNode(nodeId, {
      display_symbol: displaySymbolDraft.trim().slice(0, 1),
      remark: remarkDraft.trim(),
    });
    queryClient.setQueryData(["nodes"], (previous: NodeSummary[] | undefined) =>
      (previous ?? []).map((node) => (node.node_id === acceptedNode.node_id ? acceptedNode : node)),
    );
    setAccepting(false);
    closeAcceptDialog();
  };

  const rejectNode = async (nodeId: string) => {
    setAccepting(true);
    await api.rejectNode(nodeId);
    queryClient.setQueryData(["nodes"], (previous: NodeSummary[] | undefined) =>
      (previous ?? []).filter((node) => node.node_id !== nodeId),
    );
    setAccepting(false);
    closeAcceptDialog();
  };

  const deleteNode = async (node: NodeSummary) => {
    Modal.confirm({
      title: "删除节点",
      content: `确认删除节点 ${getNodeDisplayName(node)} 吗？`,
      okText: "删除",
      okButtonProps: { danger: true },
      cancelText: "取消",
      centered: true,
      onOk: async () => {
        await api.rejectNode(node.node_id);
        queryClient.setQueryData(["nodes"], (previous: NodeSummary[] | undefined) =>
          (previous ?? []).filter((item) => item.node_id !== node.node_id),
        );
        if (selectedEntry?.kind === "node" && selectedEntry.node_id === node.node_id) {
          setSelectedEntryId(null);
        }
      },
    });
  };

  const deletePersona = async (persona: PersonaSummary) => {
    Modal.confirm({
      title: "删除智能体",
      content: `确认删除智能体 ${persona.name} 吗？`,
      okText: "删除",
      okButtonProps: { danger: true },
      cancelText: "取消",
      onOk: async () => {
        await api.deletePersona(persona.persona_id);
        queryClient.setQueryData(
          ["personas"],
          (previous: PersonaSummary[] | undefined) =>
            (previous ?? []).filter((entry) => entry.persona_id !== persona.persona_id),
        );
        setSelectedEntryId((current) => (current === `agent:${persona.persona_id}` ? null : current));
        message.success(`已删除智能体 ${persona.name}`);
      },
    });
  };

  const startChatWithPersona = async (persona: PersonaSummary) => {
    const chat = await api.createChat({ personaIds: [persona.persona_id] });
    queryClient.setQueryData(["chats"], (previous: ChatSummary[] | undefined) => {
      const others = (previous ?? []).filter((entry) => entry.chat_id !== chat.chat_id);
      return sortChats([{ ...chat, unread_count: 0 }, ...others]);
    });
    navigate(getChatPath(chat.chat_id));
  };

  const closeCreateDialog = () => {
    const next = new URLSearchParams(searchParams);
    next.delete("create");
    setSearchParams(next, { replace: true });
  };

  const handleCreatedPersona = (persona: PersonaSummary) => {
    queryClient.setQueryData(["personas"], (previous: PersonaSummary[] | undefined) => [persona, ...(previous ?? [])]);
    setSelectedEntryId(`agent:${persona.persona_id}`);
    closeCreateDialog();
  };

  return (
    <>
      <DirectoryLayout
        title="智能体 / 节点"
        sections={sections}
        selectedId={selectedEntry?.id ?? null}
        onSelect={setSelectedEntryId}
        renderRow={(id) => {
          const entry = entries.find((item) => item.id === id);
          if (!entry) return null;
          if (entry.kind === "node") {
            return (
              <Dropdown
                trigger={["contextMenu"]}
                menu={{
                  items: [{ key: "delete", label: "删除节点", danger: true }],
                  onClick: ({ key, domEvent }) => {
                    domEvent.stopPropagation();
                    if (key === "delete") {
                      void deleteNode(entry);
                    }
                  },
                }}
              >
                <div className="directory-node-row">
                <div className="directory-avatar">{(entry.display_symbol || getNodeDisplayName(entry)).slice(0, 1)}</div>
                <div className="directory-copy">
                  <div className="directory-title-row">
                    <Typography.Text strong className="directory-name-text">
                      {getNodeDisplayName(entry)}
                    </Typography.Text>
                    {entry.can_accept ? (
                      <Button
                        size="small"
                        className="directory-accept-button"
                        onClick={(event) => {
                          event.stopPropagation();
                          openAcceptDialog(entry);
                        }}
                      >
                        接收
                      </Button>
                    ) : (
                      <Typography.Text type="secondary" className="directory-presence-text">
                        {entry.status_label}
                      </Typography.Text>
                    )}
                  </div>
                  <Typography.Text type="secondary" className="directory-meta-line">
                    {entry.hello_message || "等待节点发来打招呼用语"}
                  </Typography.Text>
                </div>
                </div>
              </Dropdown>
            );
          }
          return (
            <Dropdown
              trigger={["contextMenu"]}
              menu={{
                items: [{ key: "delete", label: "删除智能体", danger: true }],
                onClick: ({ key, domEvent }) => {
                  domEvent.stopPropagation();
                  if (key === "delete") {
                    void deletePersona(entry);
                  }
                },
              }}
            >
              <div className="directory-node-row">
                <PersonaAvatar persona={entry} className="directory-avatar agent" />
                <div className="directory-copy">
                  <Typography.Text strong>{entry.name}</Typography.Text>
                  <Typography.Text type="secondary">{entry.system_prompt || "未设置角色设定"}</Typography.Text>
                </div>
              </div>
            </Dropdown>
          );
        }}
        detail={
          selectedEntry ? (
            selectedEntry.kind === "node" ? (
              <div className="profile-card">
                <div className="profile-header">
                  <div className="profile-avatar">
                    {(selectedEntry.display_symbol || getNodeDisplayName(selectedEntry)).slice(0, 1)}
                  </div>
                  <div className="profile-headline">
                    <div className="profile-title-row">
                      <Typography.Title level={3} style={{ margin: 0 }}>
                        {getNodeDisplayName(selectedEntry)}
                      </Typography.Title>
                      <Typography.Text type="secondary" className="profile-presence-text">
                        {selectedEntry.status_label}
                      </Typography.Text>
                    </div>
                    <Typography.Text type="secondary" className="profile-subline">
                      节点名称：{getNodeName(selectedEntry)}
                    </Typography.Text>
                  </div>
                </div>
                <div className="profile-grid">
                  <div className="profile-row">
                    <span className="profile-label">节点名</span>
                    <span>{getNodeName(selectedEntry)}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">状态</span>
                    <span>{selectedEntry.status_label}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">打招呼语</span>
                    <span>{selectedEntry.hello_message || "-"}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">最后在线</span>
                    <span>{formatReadableTime(selectedEntry.last_seen_at)}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">系统</span>
                    <span>
                      {selectedEntry.platform ?? "-"} / {selectedEntry.arch ?? "-"}
                    </span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">运行中任务</span>
                    <span>{selectedEntry.running_turns ?? 0}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">备注</span>
                    <span>{selectedEntry.remark.trim() || "-"}</span>
                  </div>
                </div>
                {selectedEntry.can_accept ? (
                  <div className="profile-actions">
                    <Button className="profile-accept-button" type="primary" onClick={() => openAcceptDialog(selectedEntry)}>
                      接收
                    </Button>
                  </div>
                ) : null}
              </div>
            ) : (
              <div className="profile-card">
                <div className="profile-header">
                  <PersonaAvatar persona={selectedEntry} className="profile-avatar agent" />
                  <div className="profile-headline">
                    <Typography.Title level={3} style={{ margin: 0 }}>
                      {selectedEntry.name}
                    </Typography.Title>
                    <Typography.Text type="secondary">智能体 ID：{selectedEntry.persona_id}</Typography.Text>
                  </div>
                </div>
                <div className="profile-grid">
                  <div className="profile-row">
                    <span className="profile-label">运行节点</span>
                    <span>{selectedEntry.node_name || selectedEntry.node_id || "-"}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">工作目录</span>
                    <span className="profile-value-wrap">{selectedEntry.workspace_dir || "-"}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">角色设定</span>
                    <span className="profile-value-wrap">{selectedEntry.system_prompt || "-"}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">Agent 类型</span>
                    <span>{selectedEntry.agent_label || selectedEntry.model_provider || "codex"}</span>
                  </div>
                </div>
                <div className="profile-actions">
                  <Button
                    type="primary"
                    className="profile-action-button profile-accept-button"
                    onClick={() => void startChatWithPersona(selectedEntry)}
                  >
                    发起聊天
                  </Button>
                  <Button className="profile-action-button profile-danger-button" onClick={() => void deletePersona(selectedEntry)}>
                    删除智能体
                  </Button>
                </div>
              </div>
            )
          ) : (
            <div className="empty-state-panel">
              <Empty description="还没有智能体或节点" />
            </div>
          )
        }
      />
      <Modal
        open={pendingNode !== null}
        onCancel={closeAcceptDialog}
        footer={null}
        centered
        width={380}
        title="接收节点"
      >
        <div className="accept-dialog-body">
          <div className="accept-dialog-preview">
            <div className="accept-dialog-symbol">{(displaySymbolDraft || (pendingNode ? getNodeDisplayName(pendingNode) : "?")).slice(0, 1)}</div>
            <div>
              <Typography.Text strong>{pendingNode ? getNodeDisplayName(pendingNode) : "-"}</Typography.Text>
              <div className="accept-dialog-subtitle">{pendingNode?.node_id ?? ""}</div>
            </div>
          </div>
          <div className="accept-dialog-field">
            <Typography.Text type="secondary">符号</Typography.Text>
            <Input maxLength={1} value={displaySymbolDraft} onChange={(event) => setDisplaySymbolDraft(event.target.value.slice(0, 1))} />
          </div>
          <div className="accept-dialog-field">
            <Typography.Text type="secondary">备注</Typography.Text>
            <Input value={remarkDraft} onChange={(event) => setRemarkDraft(event.target.value)} placeholder="给这个节点写一个备注" />
          </div>
          <div className="accept-dialog-actions">
            <Button onClick={closeAcceptDialog} disabled={accepting}>
              取消
            </Button>
            <Button danger onClick={() => pendingNode && void rejectNode(pendingNode.node_id)} loading={accepting}>
              拒绝
            </Button>
            <Button type="primary" onClick={() => pendingNode && void acceptNode(pendingNode.node_id)} loading={accepting}>
              确定
            </Button>
          </div>
        </div>
      </Modal>
      <FriendWizard open={openCreate} nodes={nodes} onClose={closeCreateDialog} onCreated={handleCreatedPersona} />
    </>
  );
};

const RuntimePage = () => (
  <section className="panel-page">
    <div className="panel-card">
      <Typography.Title level={3}>运行设置</Typography.Title>
      <List
        dataSource={[
          "默认模型供应商：codex",
          "默认超时：30 秒",
          "默认自由讨论轮数：3",
          "默认每轮人数：2",
        ]}
        renderItem={(item) => <List.Item>{item}</List.Item>}
      />
    </div>
  </section>
);

const AppShell = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { data: chats = [] } = useQuery({ queryKey: ["chats"], queryFn: api.chats });
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });
  const activeChatId = useMemo(() => {
    const match = location.pathname.match(/^\/chats\/([^/]+)/);
    if (match) {
      return match[1];
    }
    if (location.pathname === "/chats") {
      return null;
    }
    return null;
  }, [location.pathname]);
  const watchedChatIds = useMemo(() => chats.map((chat) => chat.chat_id), [chats]);
  const consoleSocket = useConsoleSocket(watchedChatIds);
  const selectedKey = useMemo(() => {
    if (location.pathname === "/chats" || location.pathname.startsWith("/chats/")) {
      return "/chats";
    }
    return location.pathname;
  }, [location.pathname]);
  const pendingNodeCount = useMemo(() => getPendingNodeCount(nodes), [nodes]);
  const openCreateChat = selectedKey === "/chats" && searchParams.get("create") === "chat";

  const closeCreateChat = () => {
    const next = new URLSearchParams(searchParams);
    next.delete("create");
    setSearchParams(next, { replace: true });
  };

  const handleCreatedChat = (chat: ChatSummary) => {
    queryClient.setQueryData(["chats"], (previous: ChatSummary[] | undefined) => {
      const others = (previous ?? []).filter((entry) => entry.chat_id !== chat.chat_id);
      return sortChats([{ ...chat, unread_count: 0 }, ...others]);
    });
    closeCreateChat();
    navigate(getChatPath(chat.chat_id));
  };

  useEffect(() => {
    const handleContextMenu = (event: MouseEvent) => {
      event.preventDefault();
    };

    window.addEventListener("contextmenu", handleContextMenu);
    return () => window.removeEventListener("contextmenu", handleContextMenu);
  }, []);

  return (
    <AntApp>
      <Layout className="frame-layout">
        <aside className="nav-rail">
          <div className="brand-stamp">
            <img src="/qunce-icon-v2-bubble.svg" alt="群策图标" className="brand-icon" />
          </div>
          <div className="rail-buttons">
            {railItems.map((item) => {
              const active = selectedKey === item.key;
              return (
                <Link key={item.key} to={item.key} className={`rail-button ${active ? "active" : ""}`}>
                  {item.key === "/friends" && pendingNodeCount > 0 ? (
                    <Badge count={pendingNodeCount} size="small">
                      <span className="rail-glyph">{item.icon}</span>
                    </Badge>
                  ) : (
                    <span className="rail-glyph">{item.icon}</span>
                  )}
                </Link>
              );
            })}
          </div>
          <div className="rail-footer">
            <Link
              to={settingsRailItem.key}
              className={`rail-button ${selectedKey === settingsRailItem.key ? "active" : ""}`}
            >
              <span className="rail-glyph">{settingsRailItem.icon}</span>
            </Link>
          </div>
        </aside>

        <Layout.Content className="frame-content">
          <Routes>
            <Route path="/" element={<Navigate to="/chats" replace />} />
            <Route path="/setup" element={<SetupPage />} />
            <Route path="/chats" element={<ChatPage {...consoleSocket} />} />
            <Route path="/chats/:chatId" element={<ChatPage {...consoleSocket} />} />
            <Route path="/settings/nodes" element={<AgentDirectoryPage defaultKind="node" />} />
            <Route path="/friends" element={<AgentDirectoryPage defaultKind="agent" />} />
            <Route path="/settings/runtime" element={<RuntimePage />} />
          </Routes>
          <StartChatDialog open={openCreateChat} personas={personas} onClose={closeCreateChat} onCreated={handleCreatedChat} />
        </Layout.Content>
      </Layout>
    </AntApp>
  );
};

export const App = AppShell;


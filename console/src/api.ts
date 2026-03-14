import type {
  AddChatMembersResponse,
  ChatSnapshot,
  ChatSummary,
  CreatePersonaPayload,
  DeleteChatResponse,
  NodeSummary,
  PersonaSummary,
  RemoveChatMemberResponse,
  UpdatePersonaPayload,
  WorkspaceValidation,
} from "./types";

const CHAT_MEMBER_ACTOR_NAME = "群成员";
const CHAT_MUTE_ACTOR_NAME = "群状态";
const USER_SENDER_NAME = "你";

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

const postNodeProfile = (
  nodeId: string,
  payload: { display_symbol: string; remark: string; avatar_bg_color: string; avatar_text_color: string },
) =>
  fetchJson<NodeSummary>(`/api/nodes/${nodeId}/accept`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const api = {
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
  acceptNode: (
    nodeId: string,
    payload: { display_symbol: string; remark: string; avatar_bg_color: string; avatar_text_color: string },
  ) => postNodeProfile(nodeId, payload),
  updateNode: (
    nodeId: string,
    payload: { display_symbol: string; remark: string; avatar_bg_color: string; avatar_text_color: string },
  ) => postNodeProfile(nodeId, payload),
  rejectNode: (nodeId: string) => fetchJson<{ ok: boolean }>(`/api/nodes/${nodeId}`, { method: "DELETE" }),
  chatSnapshot: (chatId: string) => fetchJson<ChatSnapshot>(`/api/chats/${chatId}/snapshot`),
  addChatMembers: (chatId: string, payload: { personaIds: string[]; actorName?: string }) =>
    fetchJson<AddChatMembersResponse>(`/api/chats/${chatId}/members`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        persona_ids: payload.personaIds,
        actor_name: payload.actorName ?? CHAT_MEMBER_ACTOR_NAME,
      }),
    }),
  removeChatMember: (chatId: string, personaId: string, payload?: { actorName?: string }) =>
    fetchJson<RemoveChatMemberResponse>(`/api/chats/${chatId}/members/${personaId}`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ actor_name: payload?.actorName ?? CHAT_MEMBER_ACTOR_NAME }),
    }),
  deleteChat: (chatId: string) =>
    fetchJson<DeleteChatResponse>(`/api/chats/${chatId}`, {
      method: "DELETE",
    }),
  toggleChatMute: (chatId: string, muted: boolean) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/mute`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ muted, actor_name: CHAT_MUTE_ACTOR_NAME }),
    }),
  toggleChatMemberMute: (chatId: string, personaId: string, muted: boolean) =>
    fetchJson<ChatSnapshot>(`/api/chats/${chatId}/members/${personaId}/mute`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ muted, actor_name: CHAT_MEMBER_ACTOR_NAME }),
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
  updatePersona: (payload: UpdatePersonaPayload) =>
    fetchJson<PersonaSummary>("/api/personas", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ _action: "update", ...payload }),
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
      body: JSON.stringify({ content, sender_name: USER_SENDER_NAME }),
    }),
};

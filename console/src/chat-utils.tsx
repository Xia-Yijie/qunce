import type { ChatSnapshot, ChatSummary, NodeSummary, PersonaSummary } from "./types";

export const agentOptions = [{ key: "codex", label: "codex" }];

export const DEFAULT_PERSONA_AVATAR_BG = "#d9e6f8";
export const DEFAULT_PERSONA_AVATAR_TEXT = "#31547e";
export const DEFAULT_NODE_AVATAR_BG = "#dde7df";
export const DEFAULT_NODE_AVATAR_TEXT = "#335248";

export const getPersonaAvatarConfig = (
  persona: Pick<PersonaSummary, "name" | "avatar_symbol" | "avatar_bg_color" | "avatar_text_color">,
) => ({
  symbol: (persona.avatar_symbol || persona.name || "?").trim().slice(0, 1) || "?",
  backgroundColor: persona.avatar_bg_color || DEFAULT_PERSONA_AVATAR_BG,
  color: persona.avatar_text_color || DEFAULT_PERSONA_AVATAR_TEXT,
});

export const getNodeAvatarConfig = (
  node: Pick<NodeSummary, "display_symbol" | "avatar_bg_color" | "avatar_text_color" | "remark" | "name" | "hostname">,
) => ({
  symbol: (node.display_symbol || getNodeDisplayName(node) || "?").trim().slice(0, 1) || "?",
  backgroundColor: node.avatar_bg_color || DEFAULT_NODE_AVATAR_BG,
  color: node.avatar_text_color || DEFAULT_NODE_AVATAR_TEXT,
});

export const renderMutedName = (name: string, muted?: boolean) => (
  <span className="muted-name">
    {muted ? <span className="muted-name-badge">禁言</span> : null}
    <span className="muted-name-text">{name}</span>
  </span>
);

export const sortChats = (chats: ChatSummary[]) =>
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

export const applyChatSummaryFromSnapshot = (chat: ChatSummary, snapshot: ChatSnapshot): ChatSummary => ({
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

export const getPendingNodeCount = (nodes: NodeSummary[]) => nodes.filter((node) => node.can_accept).length;

export const getChatPath = (chatId: string) => `/chats/${chatId}`;

export const getChatPreview = (chat: ChatSummary) => {
  if (chat.last_message_preview?.trim()) {
    return chat.last_message_preview.trim();
  }
  return "还没有消息，等待第一轮讨论";
};

export const formatMessageTime = (raw: string) => {
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
};

export const formatConversationTime = (raw?: string | null) => {
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const now = new Date();
  const sameDay =
    now.getFullYear() === date.getFullYear() &&
    now.getMonth() === date.getMonth() &&
    now.getDate() === date.getDate();

  if (sameDay) {
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

export const formatReadableTime = (raw?: string) => {
  if (!raw) {
    return "-";
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return raw;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
};

export const getNodeName = (node: Pick<NodeSummary, "name" | "hostname">) => {
  const hostname = node.hostname.trim();
  if (hostname) {
    return hostname;
  }
  const fallback = node.name.split("@").pop()?.trim();
  return fallback || node.name;
};

export const joinWorkspacePath = (baseDir: string, name: string) => {
  const trimmedBase = baseDir.trim().replace(/[\\/]+$/, "");
  const sanitizedName = name.trim().replace(/[<>:"/\\|?*]+/g, "_") || "friend";
  if (!trimmedBase) {
    return sanitizedName;
  }
  const separator = trimmedBase.includes("\\") ? "\\" : "/";
  return `${trimmedBase}${separator}${sanitizedName}`;
};

export const getNodeDisplayName = (node: Pick<NodeSummary, "remark" | "name" | "hostname">) => {
  const remark = node.remark.trim();
  if (remark) {
    return remark;
  }
  return getNodeName(node);
};

export const isEmbeddedNode = (node: Pick<NodeSummary, "is_embedded">) => Boolean(node.is_embedded);

export const getNodeMetaText = (node: Pick<NodeSummary, "is_embedded" | "hello_message">) => {
  if (isEmbeddedNode(node)) {
    return "伴生节点";
  }
  return node.hello_message?.trim() || "等待节点发来打招呼语";
};

import { useEffect, useMemo, useState } from "react";
import {
  App as AntApp,
  Badge,
  Button,
  Empty,
  Flex,
  Input,
  Layout,
  List,
  Modal,
  Space,
  Spin,
  Typography,
} from "antd";
import { useQuery } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";

type RoomSummary = {
  room_id: string;
  name: string;
  mode: string;
  member_count: number;
  message_count: number;
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
  platform?: string;
  arch?: string;
  last_seen_at?: string;
};

type PersonaSummary = {
  persona_id: string;
  name: string;
  role_summary: string;
  status: string;
};

type RoomSnapshot = {
  room_id: string;
  name: string;
  mode: string;
  members: Array<{ persona_id: string; name: string; status: string }>;
  messages: Array<{
    message_id: string;
    sender_type: string;
    sender_name: string;
    content: string;
    status: string;
    created_at: string;
  }>;
};

const fetchJson = async <T,>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, init);
  if (!response.ok) {
    throw new Error(`request failed: ${url}`);
  }
  return response.json() as Promise<T>;
};

const api = {
  rooms: () => fetchJson<RoomSummary[]>("/api/rooms"),
  nodes: () => fetchJson<NodeSummary[]>("/api/nodes"),
  acceptNode: (nodeId: string, payload: { display_symbol: string; remark: string }) =>
    fetchJson<NodeSummary>(`/api/nodes/${nodeId}/accept`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  rejectNode: (nodeId: string) => fetchJson<{ ok: boolean }>(`/api/nodes/${nodeId}`, { method: "DELETE" }),
  roomSnapshot: (roomId: string) => fetchJson<RoomSnapshot>(`/api/rooms/${roomId}/snapshot`),
  personas: () => fetchJson<PersonaSummary[]>("/api/personas"),
  sendMessage: (roomId: string, content: string) =>
    fetchJson<{ ok: boolean }>(`/api/rooms/${roomId}/messages`, {
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

const railItems = [
  { key: "/rooms/room_lobby", icon: <ChatRailIcon />, description: "聊天" },
  { key: "/settings/personas", icon: <AgentRailIcon />, description: "Agent" },
];

const settingsRailItem = { key: "/settings/runtime", icon: <SettingsRailIcon />, description: "设置" };

const getPendingNodeCount = (nodes: NodeSummary[]) => nodes.filter((node) => node.can_accept).length;

const useConsoleSocket = (roomId: string) => {
  const queryClient = useQueryClient();
  const [connected, setConnected] = useState(false);
  const [lastNotice, setLastNotice] = useState("等待连接群聊通道");

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/console`);

    socket.addEventListener("open", () => {
      setConnected(true);
      setLastNotice("已连接群聊通道");
      socket.send(
        JSON.stringify({
          v: 1,
          type: "console.subscribe",
          event_id: `evt_${crypto.randomUUID()}`,
          request_id: `req_${crypto.randomUUID()}`,
          ts: new Date().toISOString(),
          source: { kind: "console", id: "browser" },
          target: { kind: "server", id: "main" },
          data: { room_ids: [roomId], watch_nodes: true },
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
      if (payload.type === "server.room.snapshot") {
        const data = payload.data as RoomSnapshot;
        queryClient.setQueryData(["room", data.room_id], data);
        queryClient.setQueryData(["rooms"], (previous: RoomSummary[] | undefined) =>
          (previous ?? []).map((room) =>
            room.room_id === data.room_id
              ? {
                  ...room,
                  member_count: data.members.length,
                  message_count: data.messages.length,
                }
              : room,
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
  }, [queryClient, roomId]);

  return { connected, lastNotice };
};

const getRoomPreview = (room: RoomSummary) => {
  if (room.message_count > 0) {
    return `${room.member_count} 个成员，最近有新消息`;
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

const getNodeFullName = (node: Pick<NodeSummary, "name" | "hostname">) =>
  node.name.includes("@") || !node.hostname ? node.name : `${node.name}@${node.hostname}`;

const getNodePrimaryName = (node: Pick<NodeSummary, "remark" | "name">) =>
  node.remark || node.name.split("@")[0] || node.name;

const getMessageTone = (senderType: string) => {
  if (senderType === "system") {
    return "system";
  }
  if (senderType === "user") {
    return "user";
  }
  return "agent";
};

const ConversationList = ({
  rooms,
  selectedRoomId,
}: {
  rooms: RoomSummary[];
  selectedRoomId: string;
}) => (
  <aside className="conversation-pane">
    <div className="conversation-header">
      <div className="conversation-search-row">
        <div className="conversation-search">搜索</div>
        <button type="button" className="conversation-plus">
          +
        </button>
      </div>
    </div>
    <List
      className="conversation-list"
      dataSource={rooms}
      renderItem={(room) => {
        const isActive = room.room_id === selectedRoomId;
        return (
          <List.Item className={`conversation-row ${isActive ? "active" : ""}`}>
            <Link to={`/rooms/${room.room_id}`} className="conversation-link">
              <div className="conversation-avatar">{room.name.slice(0, 1)}</div>
              <div className="conversation-copy">
                <Flex justify="space-between" align="center" gap={12}>
                  <Typography.Text strong>{room.name}</Typography.Text>
                  <Typography.Text type="secondary">{room.message_count > 0 ? "刚刚" : ""}</Typography.Text>
                </Flex>
                <Typography.Text type="secondary" className="conversation-preview">
                  {getRoomPreview(room)}
                </Typography.Text>
              </div>
            </Link>
          </List.Item>
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
          <div className="directory-search">搜索</div>
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
                  <span className={`directory-section-caret ${collapsed ? "collapsed" : ""}`}>⌄</span>
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
}: {
  senderName: string;
  senderType: string;
  content: string;
  createdAt: string;
}) => {
  const tone = getMessageTone(senderType);

  return (
    <div className={`message-row ${tone}`}>
      <div className={`message-avatar ${tone}`}>{senderName.slice(0, 1)}</div>
      <div className="message-body">
        <Flex align="center" gap={10}>
          <Typography.Text strong>{senderName}</Typography.Text>
          <Typography.Text type="secondary" className="message-time">
            {formatMessageTime(createdAt)}
          </Typography.Text>
        </Flex>
        <div className={`message-bubble ${tone}`}>{content}</div>
      </div>
    </div>
  );
};

const RoomPage = ({ connected, lastNotice }: { connected: boolean; lastNotice: string }) => {
  const { roomId = "room_lobby" } = useParams();
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const { data: rooms = [] } = useQuery({ queryKey: ["rooms"], queryFn: api.rooms });
  const { data: snapshot, isLoading } = useQuery({
    queryKey: ["room", roomId],
    queryFn: () => api.roomSnapshot(roomId),
  });

  const sendMessage = async () => {
    const content = draft.trim();
    if (!content || sending) {
      return;
    }
    setSending(true);
    try {
      await api.sendMessage(roomId, content);
      setDraft("");
    } finally {
      setSending(false);
    }
  };

  if (isLoading) {
    return (
      <div className="wechat-shell loading-shell">
        <Spin />
      </div>
    );
  }

  if (!snapshot) {
    return (
      <div className="wechat-shell">
        <ConversationList rooms={rooms} selectedRoomId={roomId} />
        <div className="chat-pane empty-pane">
          <Empty description="房间不存在" />
        </div>
      </div>
    );
  }

  return (
    <div className="wechat-shell">
      <ConversationList rooms={rooms} selectedRoomId={roomId} />

      <section className="chat-pane">
        <header className="chat-header">
          <div>
            <Typography.Title level={4} style={{ margin: 0 }}>
              {snapshot.name}
            </Typography.Title>
            <Space size={10} wrap>
              <Badge status={connected ? "success" : "default"} text={connected ? "在线" : "离线"} />
              <Typography.Text type="secondary">{lastNotice}</Typography.Text>
            </Space>
          </div>
          <Space>
            <Button type="text">···</Button>
          </Space>
        </header>

        <div className="message-stream">
          {snapshot.messages.map((message) => (
            <MessageBubble
              key={message.message_id}
              senderName={message.sender_name}
              senderType={message.sender_type}
              content={message.content}
              createdAt={message.created_at}
            />
          ))}
        </div>

        <footer className="composer-panel">
          <div className="composer-toolbar">
            <Button type="text">☺</Button>
            <Button type="text">@</Button>
            <Button type="text">□</Button>
            <Button type="text">✂</Button>
            <Button type="text">⌄</Button>
            <Button type="text">◉</Button>
          </div>
          <Input.TextArea
            autoSize={{ minRows: 4, maxRows: 7 }}
            bordered={false}
            placeholder="输入消息，后续这里会接真实发送能力"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onPressEnter={(event) => {
              if (!event.shiftKey) {
                event.preventDefault();
                void sendMessage();
              }
            }}
          />
          <Flex justify="space-between" align="center">
            <Typography.Text type="secondary">点名模式</Typography.Text>
            <Button className="send-button" type="default" onClick={() => void sendMessage()} loading={sending}>
              发送(S)
            </Button>
          </Flex>
        </footer>
      </section>
    </div>
  );
};

const SetupPage = () => {
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });

  return (
    <section className="panel-page">
      <div className="panel-card">
        <Typography.Title level={3}>配置向导</Typography.Title>
        <Typography.Paragraph>
          先在受信环境里启动本地 agent，完成服务端配对后，再确认节点和身份已经准备好，随后进入群聊房间。
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

type DirectoryEntry =
  | ({ kind: "agent"; id: string } & PersonaSummary)
  | ({ kind: "node"; id: string } & NodeSummary);

const AgentDirectoryPage = ({ defaultKind }: { defaultKind: "agent" | "node" }) => {
  const queryClient = useQueryClient();
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

  const openAcceptDialog = (node: NodeSummary) => {
    setPendingNodeId(node.node_id);
    setDisplaySymbolDraft(node.display_symbol || getNodeFullName(node).slice(0, 1));
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
              <>
                <div className="directory-avatar">{(entry.display_symbol || getNodeFullName(entry)).slice(0, 1)}</div>
                <div className="directory-copy">
                  <div className="directory-title-row">
                    <Typography.Text strong>{getNodeFullName(entry)}</Typography.Text>
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
              </>
            );
          }
          return (
            <>
              <div className="directory-avatar agent">{entry.name.slice(0, 1)}</div>
              <div className="directory-copy">
                <Typography.Text strong>{entry.name}</Typography.Text>
                <Typography.Text type="secondary">{entry.role_summary}</Typography.Text>
              </div>
            </>
          );
        }}
        detail={
          selectedEntry ? (
            selectedEntry.kind === "node" ? (
              <div className="profile-card">
                <div className="profile-header">
                  <div className="profile-avatar">{(selectedEntry.display_symbol || getNodeFullName(selectedEntry)).slice(0, 1)}</div>
                  <div className="profile-headline">
                    <div className="profile-title-row">
                      <Typography.Title level={3} style={{ margin: 0 }}>
                        {getNodePrimaryName(selectedEntry)}
                      </Typography.Title>
                      <Typography.Text type="secondary" className="profile-presence-text">
                        {selectedEntry.status_label}
                      </Typography.Text>
                    </div>
                    <Typography.Text type="secondary" className="profile-subline">
                      节点名：{getNodeFullName(selectedEntry)}
                    </Typography.Text>
                  </div>
                </div>
                <div className="profile-grid">
                  <div className="profile-row">
                    <span className="profile-label">备注/用户名</span>
                    <span>{getNodePrimaryName(selectedEntry)}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">节点名</span>
                    <span>{getNodeFullName(selectedEntry)}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">打招呼语</span>
                    <span>{selectedEntry.hello_message || "-"}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">状态</span>
                    <span>{selectedEntry.status_label}</span>
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
                  <div className="profile-avatar agent">{selectedEntry.name.slice(0, 1)}</div>
                  <div className="profile-headline">
                    <Typography.Title level={3} style={{ margin: 0 }}>
                      {selectedEntry.name}
                    </Typography.Title>
                    <Typography.Text type="secondary">智能体 ID：{selectedEntry.persona_id}</Typography.Text>
                  </div>
                </div>
                <div className="profile-grid">
                  <div className="profile-row">
                    <span className="profile-label">状态</span>
                    <span>{selectedEntry.status}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">角色说明</span>
                    <span>{selectedEntry.role_summary}</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">运行节点</span>
                    <span>后续接入</span>
                  </div>
                  <div className="profile-row">
                    <span className="profile-label">模型</span>
                    <span>codex</span>
                  </div>
                </div>
              </div>
            )
          ) : (
            <Empty description="还没有智能体或节点" />
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
            <div className="accept-dialog-symbol">{(displaySymbolDraft || (pendingNode ? getNodeFullName(pendingNode) : "•")).slice(0, 1)}</div>
            <div>
              <Typography.Text strong>{pendingNode ? getNodeFullName(pendingNode) : "-"}</Typography.Text>
              <div className="accept-dialog-subtitle">{pendingNode?.node_id ?? ""}</div>
            </div>
          </div>
          <div className="accept-dialog-field">
            <Typography.Text type="secondary">符号</Typography.Text>
            <Input maxLength={1} value={displaySymbolDraft} onChange={(event) => setDisplaySymbolDraft(event.target.value.slice(0, 1))} />
          </div>
          <div className="accept-dialog-field">
            <Typography.Text type="secondary">备注</Typography.Text>
            <Input value={remarkDraft} onChange={(event) => setRemarkDraft(event.target.value)} placeholder="给这个节点写个备注" />
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
          "默认超时：300 秒",
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
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const activeRoomId = useMemo(() => {
    const match = location.pathname.match(/^\/rooms\/([^/]+)/);
    return match?.[1] ?? "room_lobby";
  }, [location.pathname]);
  const consoleSocket = useConsoleSocket(activeRoomId);
  const selectedKey = useMemo(() => {
    if (location.pathname.startsWith("/rooms")) {
      return "/rooms/room_lobby";
    }
    return location.pathname;
  }, [location.pathname]);
  const pendingNodeCount = useMemo(() => getPendingNodeCount(nodes), [nodes]);

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
                  {item.key === "/settings/personas" && pendingNodeCount > 0 ? (
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
            <Route path="/" element={<Navigate to="/rooms/room_lobby" replace />} />
            <Route path="/setup" element={<SetupPage />} />
            <Route path="/rooms/:roomId" element={<RoomPage {...consoleSocket} />} />
            <Route path="/settings/nodes" element={<AgentDirectoryPage defaultKind="node" />} />
            <Route path="/settings/personas" element={<AgentDirectoryPage defaultKind="agent" />} />
            <Route path="/settings/runtime" element={<RuntimePage />} />
          </Routes>
        </Layout.Content>
      </Layout>
    </AntApp>
  );
};

export const App = AppShell;

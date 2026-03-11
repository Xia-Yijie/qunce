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
  Space,
  Spin,
  Typography,
} from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link, Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";

type AppMeta = {
  name: string;
  version: string;
  default_pair_token: string;
};

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
  status: string;
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

const fetchJson = async <T,>(url: string): Promise<T> => {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`request failed: ${url}`);
  }
  return response.json() as Promise<T>;
};

const api = {
  meta: () => fetchJson<AppMeta>("/api/meta"),
  rooms: () => fetchJson<RoomSummary[]>("/api/rooms"),
  nodes: () => fetchJson<NodeSummary[]>("/api/nodes"),
  roomSnapshot: (roomId: string) => fetchJson<RoomSnapshot>(`/api/rooms/${roomId}/snapshot`),
  personas: () => fetchJson<PersonaSummary[]>("/api/personas"),
};

const railItems = [
  { key: "/rooms/room_lobby", glyph: "●", description: "群聊" },
  { key: "/setup", glyph: "◎", description: "配对" },
  { key: "/settings/nodes", glyph: "◌", description: "节点" },
  { key: "/settings/personas", glyph: "◐", description: "身份" },
];

const settingsRailItem = { key: "/settings/runtime", glyph: "☰", description: "设置" };

const useConsoleSocket = (roomId: string) => {
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
          data: { room_ids: [roomId], watch_nodes: true, watch_turns: true },
        }),
      );
    });

    socket.addEventListener("message", (event) => {
      const payload = JSON.parse(event.data) as { type: string; data?: Record<string, unknown> };
      if (payload.type === "server.notice") {
        setLastNotice(String(payload.data?.message ?? "收到服务器通知"));
      }
      if (payload.type === "server.node.updated") {
        setLastNotice("节点状态已同步");
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
  }, [roomId]);

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

const RoomPage = () => {
  const { roomId = "room_lobby" } = useParams();
  const { connected, lastNotice } = useConsoleSocket(roomId);
  const { data: rooms = [] } = useQuery({ queryKey: ["rooms"], queryFn: api.rooms });
  const { data: snapshot, isLoading } = useQuery({
    queryKey: ["room", roomId],
    queryFn: () => api.roomSnapshot(roomId),
  });

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
          />
          <Flex justify="space-between" align="center">
            <Typography.Text type="secondary">点名模式</Typography.Text>
            <Button className="send-button" type="default">
              发送(S)
            </Button>
          </Flex>
        </footer>
      </section>
    </div>
  );
};

const SetupPage = () => {
  const { data: meta } = useQuery({ queryKey: ["meta"], queryFn: api.meta });
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });

  return (
    <section className="panel-page">
      <div className="panel-card">
        <Typography.Title level={3}>配置向导</Typography.Title>
        <Typography.Paragraph>
          先启动本地 agent，再确认配对码、节点和身份已经准备好，随后进入群聊房间。
        </Typography.Paragraph>
        <List
          dataSource={[
            `默认配对码：${meta?.default_pair_token ?? "-"}`,
            `当前在线节点：${nodes.filter((node) => node.status === "online").length}`,
            `已配置身份：${personas.length}`,
            "下一步：进入群聊主界面发起第一轮讨论",
          ]}
          renderItem={(item) => <List.Item>{item}</List.Item>}
        />
      </div>
    </section>
  );
};

const NodesPage = () => {
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: api.nodes });

  return (
    <section className="panel-page">
      <div className="panel-card">
        <Typography.Title level={3}>节点</Typography.Title>
        <List
          dataSource={nodes}
          locale={{ emptyText: "还没有节点上线，先启动 Local Agent" }}
          renderItem={(node) => (
            <List.Item>
              <Flex justify="space-between" align="center" style={{ width: "100%" }}>
                <div>
                  <Typography.Text strong>{node.name}</Typography.Text>
                  <div>
                    <Typography.Text type="secondary">
                      {node.platform}/{node.arch}
                    </Typography.Text>
                  </div>
                </div>
                <Typography.Text type={node.status === "online" ? undefined : "secondary"}>
                  {node.status}
                </Typography.Text>
              </Flex>
            </List.Item>
          )}
        />
      </div>
    </section>
  );
};

const PersonasPage = () => {
  const { data: personas = [] } = useQuery({ queryKey: ["personas"], queryFn: api.personas });

  return (
    <section className="panel-page">
      <div className="panel-card">
        <Typography.Title level={3}>身份</Typography.Title>
        <List
          dataSource={personas}
          renderItem={(persona) => (
            <List.Item>
              <div>
                <Typography.Text strong>{persona.name}</Typography.Text>
                <div>
                  <Typography.Text type="secondary">{persona.role_summary}</Typography.Text>
                </div>
              </div>
            </List.Item>
          )}
        />
      </div>
    </section>
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
  const selectedKey = useMemo(() => {
    if (location.pathname.startsWith("/rooms")) {
      return "/rooms/room_lobby";
    }
    return location.pathname;
  }, [location.pathname]);

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
                  <span className="rail-glyph">{item.glyph}</span>
                </Link>
              );
            })}
          </div>
          <div className="rail-footer">
            <Link
              to={settingsRailItem.key}
              className={`rail-button ${selectedKey === settingsRailItem.key ? "active" : ""}`}
            >
              <span className="rail-glyph">{settingsRailItem.glyph}</span>
            </Link>
          </div>
        </aside>

        <Layout.Content className="frame-content">
          <Routes>
            <Route path="/" element={<Navigate to="/rooms/room_lobby" replace />} />
            <Route path="/setup" element={<SetupPage />} />
            <Route path="/rooms/:roomId" element={<RoomPage />} />
            <Route path="/settings/nodes" element={<NodesPage />} />
            <Route path="/settings/personas" element={<PersonasPage />} />
            <Route path="/settings/runtime" element={<RuntimePage />} />
          </Routes>
        </Layout.Content>
      </Layout>
    </AntApp>
  );
};

export const App = AppShell;

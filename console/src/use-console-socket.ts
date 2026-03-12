import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { applyChatSummaryFromSnapshot, sortChats } from "./chat-utils";
import type { ChatSnapshot, ChatSummary, NodeSummary } from "./types";

export const useConsoleSocket = (chatIds: string[]) => {
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
        setLastNotice(String(data.message ?? "收到服务端通知"));
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

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { applyChatSummaryFromSnapshot, chatSummaryFromSnapshot, sortChats } from "./chat-utils";
import type { ChatSnapshot, ChatSummary, NodeSummary } from "./types";

export const useConsoleSocket = (chatIds: string[]) => {
  const queryClient = useQueryClient();

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/console`);

    socket.addEventListener("open", () => {
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
      if (payload.type === "server.node.updated") {
        const data = (payload.data ?? {}) as { nodes?: NodeSummary[] };
        queryClient.setQueryData(["nodes"], data.nodes ?? []);
      }
      if (payload.type === "server.chat.snapshot") {
        const data = payload.data as ChatSnapshot;
        queryClient.setQueryData(["chat", data.chat_id], data);
        queryClient.setQueryData(["chats"], (previous: ChatSummary[] | undefined) => {
          const chats = previous ?? [];
          const existing = chats.find((chat) => chat.chat_id === data.chat_id);
          if (existing) {
            return sortChats(
              chats.map((chat) => (chat.chat_id === data.chat_id ? applyChatSummaryFromSnapshot(chat, data) : chat)),
            );
          }

          const appended: ChatSummary = chatSummaryFromSnapshot(data);
          return sortChats([appended, ...chats]);
        });
      }
    });

    return () => socket.close();
  }, [chatIds.join("|"), queryClient]);
};

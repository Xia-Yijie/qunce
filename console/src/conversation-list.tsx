import { useMemo, useState } from "react";
import { Badge, Dropdown, Flex, List, Typography } from "antd";
import type { MenuProps } from "antd";
import { Link, useNavigate } from "react-router-dom";

import { formatConversationTime, getChatPath, getChatPreview } from "./chat-utils";
import { SearchInputDropdown } from "./search-input-dropdown";
import type { ChatSummary } from "./types";

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
    <path
      d="M3.8 15C4.2 12.9 5.9 11.5 8 11.5C10.1 11.5 11.8 12.9 12.2 15"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
    />
    <path d="M15 6V10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    <path d="M13 8H17" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const QuickCreateMenu = () => {
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
                <span>发起聊天</span>
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

export const ConversationList = ({
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
}) => {
  const navigate = useNavigate();
  const [keyword, setKeyword] = useState("");

  const chatResults = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    if (!query) {
      return [];
    }
    return chats.filter((chat) => {
      const haystack = `${chat.name} ${getChatPreview(chat)}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [chats, keyword]);

  return (
    <aside className="conversation-pane">
      <div className="conversation-header">
        <div className="conversation-search-row">
          <SearchInputDropdown
            value={keyword}
            onChange={setKeyword}
            placeholder="搜索"
            storageKey="qunce.search.chats"
            title="搜索聊天结果"
            subtitle="聊天名称、消息内容等"
            emptyText="没有最近搜索内容"
            resultEmptyText="没有匹配的聊天"
            results={chatResults.map((chat) => ({
              key: chat.chat_id,
              title: chat.name,
              subtitle: getChatPreview(chat),
              onSelect: () => navigate(getChatPath(chat.chat_id)),
            }))}
            className="conversation-search"
          />
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
                      <Typography.Text type="secondary">
                        {formatConversationTime(chat.last_message_at)}
                      </Typography.Text>
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
};

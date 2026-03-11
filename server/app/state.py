from __future__ import annotations

from copy import deepcopy
from datetime import UTC, datetime
from threading import Lock
from uuid import uuid4


class InMemoryState:
    def __init__(self) -> None:
        self._lock = Lock()
        self.nodes: dict[str, dict] = {}
        self.turns: dict[str, dict] = {}
        self.personas: list[dict] = []
        self.chats: dict[str, dict] = {}

    def upsert_node(self, node_id: str, payload: dict) -> dict:
        with self._lock:
            self.nodes[node_id] = {
                **self.nodes.get(node_id, {}),
                **payload,
                "last_seen_at": datetime.now(tz=UTC).isoformat(),
            }
            return deepcopy(self.nodes[node_id])

    def list_nodes(self) -> list[dict]:
        with self._lock:
            return [deepcopy(node) for node in self.nodes.values()]

    def get_node(self, node_id: str) -> dict | None:
        with self._lock:
            node = self.nodes.get(node_id)
            if node is None:
                return None
            return deepcopy(node)

    def delete_node(self, node_id: str) -> dict | None:
        with self._lock:
            node = self.nodes.pop(node_id, None)
            if node is None:
                return None
            return deepcopy(node)

    def list_personas(self) -> list[dict]:
        return deepcopy(self.personas)

    def add_chat_message(
        self,
        chat_id: str,
        *,
        sender_type: str,
        sender_name: str,
        content: str,
        status: str = "completed",
        metadata: dict | None = None,
    ) -> dict | None:
        with self._lock:
            chat = self.chats.get(chat_id)
            if chat is None:
                return None
            chat["messages"].append(
                {
                    "message_id": f"msg_{uuid4().hex}",
                    "sender_type": sender_type,
                    "sender_name": sender_name,
                    "content": content,
                    "status": status,
                    "created_at": datetime.now(tz=UTC).isoformat(),
                    "metadata": metadata or {},
                }
            )
            return deepcopy(chat)

    def create_turn(self, chat_id: str, *, content: str, node_id: str | None, sender_name: str) -> dict | None:
        with self._lock:
            chat = self.chats.get(chat_id)
            if chat is None:
                return None
            turn_id = f"turn_{uuid4().hex}"
            turn = {
                "turn_id": turn_id,
                "chat_id": chat_id,
                "content": content,
                "status": "queued",
                "assigned_node_id": node_id,
                "sender_name": sender_name,
                "created_at": datetime.now(tz=UTC).isoformat(),
                "updated_at": datetime.now(tz=UTC).isoformat(),
            }
            self.turns[turn_id] = turn
            chat.setdefault("turns", []).append(turn_id)
            return deepcopy(turn)

    def update_turn(self, turn_id: str, payload: dict) -> dict | None:
        with self._lock:
            turn = self.turns.get(turn_id)
            if turn is None:
                return None
            turn.update(payload)
            turn["updated_at"] = datetime.now(tz=UTC).isoformat()
            return deepcopy(turn)

    def get_turn(self, turn_id: str) -> dict | None:
        with self._lock:
            turn = self.turns.get(turn_id)
            if turn is None:
                return None
            return deepcopy(turn)

    def chat_snapshot(self, chat_id: str) -> dict | None:
        with self._lock:
            chat = self.chats.get(chat_id)
            if chat is None:
                return None
            return deepcopy(chat)


state = InMemoryState()

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
        self.personas: list[dict] = [
            {
                "persona_id": "persona_architect",
                "name": "架构师",
                "role_summary": "负责从分层、边界和风险角度给出建议。",
                "status": "active",
            },
            {
                "persona_id": "persona_builder",
                "name": "执行者",
                "role_summary": "负责把方案收敛为可以直接开工的任务。",
                "status": "active",
            },
        ]
        self.rooms: dict[str, dict] = {
            "room_lobby": {
                "room_id": "room_lobby",
                "name": "产品讨论组",
                "mode": "mention",
                "members": [
                    {"persona_id": "persona_architect", "name": "架构师", "status": "idle"},
                    {"persona_id": "persona_builder", "name": "执行者", "status": "idle"},
                ],
                "messages": [
                    {
                        "message_id": "msg_seed_1",
                        "sender_type": "system",
                        "sender_name": "系统",
                        "content": "欢迎来到群策。先完成本地节点配对，再开始群聊式协作。",
                        "status": "completed",
                        "created_at": datetime.now(tz=UTC).isoformat(),
                    }
                ],
                "turns": [],
            }
        }

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

    def add_room_message(
        self,
        room_id: str,
        *,
        sender_type: str,
        sender_name: str,
        content: str,
        status: str = "completed",
        metadata: dict | None = None,
    ) -> dict | None:
        with self._lock:
            room = self.rooms.get(room_id)
            if room is None:
                return None
            room["messages"].append(
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
            return deepcopy(room)

    def create_turn(self, room_id: str, *, content: str, node_id: str | None, sender_name: str) -> dict | None:
        with self._lock:
            room = self.rooms.get(room_id)
            if room is None:
                return None
            turn_id = f"turn_{uuid4().hex}"
            turn = {
                "turn_id": turn_id,
                "room_id": room_id,
                "content": content,
                "status": "queued",
                "assigned_node_id": node_id,
                "sender_name": sender_name,
                "created_at": datetime.now(tz=UTC).isoformat(),
                "updated_at": datetime.now(tz=UTC).isoformat(),
            }
            self.turns[turn_id] = turn
            room.setdefault("turns", []).append(turn_id)
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

    def room_snapshot(self, room_id: str) -> dict | None:
        with self._lock:
            room = self.rooms.get(room_id)
            if room is None:
                return None
            return deepcopy(room)


state = InMemoryState()

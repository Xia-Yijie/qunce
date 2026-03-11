from __future__ import annotations

from copy import deepcopy
from datetime import UTC, datetime
from threading import Lock


class InMemoryState:
    def __init__(self) -> None:
        self._lock = Lock()
        self.nodes: dict[str, dict] = {}
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

    def list_personas(self) -> list[dict]:
        return deepcopy(self.personas)

    def list_rooms(self) -> list[dict]:
        return [
            {
                "room_id": room["room_id"],
                "name": room["name"],
                "mode": room["mode"],
                "member_count": len(room["members"]),
                "message_count": len(room["messages"]),
            }
            for room in self.rooms.values()
        ]

    def room_snapshot(self, room_id: str) -> dict | None:
        room = self.rooms.get(room_id)
        if room is None:
            return None
        return deepcopy(room)


state = InMemoryState()

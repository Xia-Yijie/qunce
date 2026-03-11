from __future__ import annotations

from threading import Lock
from typing import Any

from server.app.protocol import envelope, send_json


class AgentConnectionRegistry:
    def __init__(self) -> None:
        self._lock = Lock()
        self._connections: dict[str, dict[str, Any]] = {}

    def upsert(self, node_id: str, ws: Any) -> None:
        with self._lock:
            self._connections[node_id] = {
                "ws": ws,
                "send_lock": Lock(),
            }

    def remove(self, node_id: str) -> None:
        with self._lock:
            self._connections.pop(node_id, None)

    def get(self, node_id: str) -> dict[str, Any] | None:
        with self._lock:
            return self._connections.get(node_id)

    def disconnect(self, node_id: str) -> None:
        connection = self.get(node_id)
        if connection is None:
            return

        try:
            with connection["send_lock"]:
                send_json(
                    connection["ws"],
                    envelope(
                        "server.notice",
                        {"level": "warning", "message": "node removed by server"},
                        source_kind="server",
                        source_id="main",
                        target_kind="agent",
                        target_id=node_id,
                    ),
                )
        except Exception:
            pass

        try:
            connection["ws"].close()
        except Exception:
            pass
        finally:
            self.remove(node_id)


agent_connections = AgentConnectionRegistry()


def dispatch_turn_to_agent(*, node_id: str, turn: dict[str, Any]) -> None:
    connection = agent_connections.get(node_id)
    if connection is None:
        raise ConnectionError(f"agent {node_id} is offline")

    payload = envelope(
        "server.turn.request",
        {
            "turn_id": turn["turn_id"],
            "chat_id": turn["chat_id"],
            "content": turn["content"],
            "sender_name": turn["sender_name"],
        },
        source_kind="server",
        source_id="main",
        target_kind="agent",
        target_id=node_id,
    )

    try:
        with connection["send_lock"]:
            send_json(connection["ws"], payload)
    except Exception as exc:
        agent_connections.remove(node_id)
        raise ConnectionError(f"agent {node_id} send failed") from exc

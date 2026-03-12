from __future__ import annotations

from threading import Event, Lock
from typing import Any
from uuid import uuid4

from server.app.protocol import envelope, send_json


class AgentConnectionRegistry:
    def __init__(self) -> None:
        self._lock = Lock()
        self._connections: dict[str, dict[str, Any]] = {}
        self._pending_workspace_checks: dict[str, dict[str, Any]] = {}

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

    def request_workspace_validation(self, *, node_id: str, workspace_dir: str, timeout: float = 8.0) -> dict[str, Any]:
        connection = self.get(node_id)
        if connection is None:
            raise ConnectionError(f"agent {node_id} is offline")

        request_id = f"req_{uuid4().hex}"
        pending = {"event": Event(), "result": None}
        with self._lock:
            self._pending_workspace_checks[request_id] = pending

        payload = envelope(
            "server.workspace.validate",
            {"workspace_dir": workspace_dir},
            source_kind="server",
            source_id="main",
            target_kind="agent",
            target_id=node_id,
            request_id=request_id,
        )

        try:
            with connection["send_lock"]:
                send_json(connection["ws"], payload)
        except Exception as exc:
            with self._lock:
                self._pending_workspace_checks.pop(request_id, None)
            self.remove(node_id)
            raise ConnectionError(f"agent {node_id} send failed") from exc

        if not pending["event"].wait(timeout):
            with self._lock:
                self._pending_workspace_checks.pop(request_id, None)
            raise TimeoutError(f"agent {node_id} workspace validation timed out")

        result = pending["result"]
        if not isinstance(result, dict):
            raise TimeoutError(f"agent {node_id} workspace validation missing result")
        return result

    def resolve_workspace_validation(self, request_id: str, result: dict[str, Any]) -> bool:
        with self._lock:
            pending = self._pending_workspace_checks.pop(request_id, None)
        if pending is None:
            return False
        pending["result"] = result
        pending["event"].set()
        return True


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
            "message_id": turn.get("message_id"),
            "content": turn["content"],
            "sender_name": turn["sender_name"],
            "persona_id": turn.get("persona_id"),
            "persona_name": turn.get("persona_name"),
            "workspace_dir": turn.get("workspace_dir"),
            "system_prompt": turn.get("system_prompt"),
            "agent_key": turn.get("agent_key"),
            "agent_label": turn.get("agent_label"),
            "muted": bool(turn.get("muted", False)),
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

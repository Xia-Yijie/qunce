from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

from server.app.config import settings
from server.app.protocol import envelope, receive_json, send_json
from server.app.runtime import sock
from server.app.state import state
from server.app.services.agents import agent_connections
from server.app.services.events import update_node
from server.app.services.turns import mark_turn_completed, mark_turn_started


def running_turn_count(payload: dict[str, Any]) -> int:
    running_turn_ids = payload.get("running_turn_ids")
    if isinstance(running_turn_ids, list):
        return len(running_turn_ids)

    legacy_running_turns = payload.get("running_turns")
    if isinstance(legacy_running_turns, list):
        return len(legacy_running_turns)
    if isinstance(legacy_running_turns, int):
        return legacy_running_turns
    return 0


def connection_status(node_id: str) -> str:
    node = state.get_node(node_id) or {}
    if not node.get("approved", False):
        return "pending"
    return "online"


@sock.route("/ws/agent")
def agent_socket(ws: Any) -> None:
    node_id = "unbound"

    try:
        hello_message = receive_json(ws)
        if hello_message.get("type") != "agent.hello":
            send_json(
                ws,
                envelope(
                    "server.error",
                    {"code": "INVALID_HANDSHAKE", "message": "expected agent.hello"},
                    source_kind="server",
                    source_id="main",
                    target_kind="agent",
                    target_id=node_id,
                ),
            )
            return

        send_json(
            ws,
            envelope(
                "server.hello",
                {
                    "server_version": settings.app_version,
                    "heartbeat_sec": 15,
                    "resume_supported": False,
                },
                source_kind="server",
                source_id="main",
                target_kind="agent",
                target_id=node_id,
                request_id=hello_message.get("request_id"),
            ),
        )

        auth_message = receive_json(ws)
        token = auth_message.get("data", {}).get("pair_token")
        if auth_message.get("type") != "agent.auth" or token != settings.default_pair_token:
            send_json(
                ws,
                envelope(
                    "server.auth.reject",
                    {"code": "PAIR_TOKEN_INVALID", "message": "配对令牌无效或已过期"},
                    source_kind="server",
                    source_id="main",
                    target_kind="agent",
                    target_id=node_id,
                    request_id=auth_message.get("request_id"),
                ),
            )
            return

        hello_payload = hello_message.get("data", {})
        hostname = str(hello_payload.get("hostname", "local-node"))
        username = str(hello_payload.get("username", hostname))
        node_id = auth_message.get("data", {}).get("node_id") or f"node_{hostname}"
        existing_node = state.get_node(node_id) or {}
        agent_connections.upsert(node_id, ws)
        update_node(
            node_id,
            {
                "node_id": node_id,
                "name": username,
                "hostname": hostname,
                "platform": hello_payload.get("platform"),
                "arch": hello_payload.get("arch"),
                "agent_version": hello_payload.get("agent_version"),
                "hello_message": hello_payload.get("hello_message") or existing_node.get("hello_message", ""),
                "approved": bool(existing_node.get("approved", False)),
                "status": "online" if existing_node.get("approved", False) else "pending",
                "running_turns": 0,
                "worker_count": 0,
            },
        )

        send_json(
            ws,
            envelope(
                "server.auth.ok",
                {
                    "node_id": node_id,
                    "node_name": username,
                    "max_workers": 2,
                },
                source_kind="server",
                source_id="main",
                target_kind="agent",
                target_id=node_id,
                request_id=auth_message.get("request_id"),
            ),
        )

        while True:
            message = receive_json(ws)
            message_type = message.get("type")
            payload = message.get("data", {})

            if message_type == "agent.state.report":
                update_node(
                    node_id,
                    {
                        "status": connection_status(node_id),
                        "running_turns": running_turn_count(payload),
                        "worker_count": payload.get("worker_count", running_turn_count(payload)),
                    },
                )
                continue

            if message_type == "agent.ping":
                update_node(
                    node_id,
                    {
                        "status": connection_status(node_id),
                        "running_turns": running_turn_count(payload),
                        "worker_count": payload.get("worker_count", 0),
                    },
                )
                send_json(
                    ws,
                    envelope(
                        "server.pong",
                        {"server_time": datetime.now(tz=UTC).isoformat()},
                        source_kind="server",
                        source_id="main",
                        target_kind="agent",
                        target_id=node_id,
                        request_id=message.get("request_id"),
                    ),
                )
                continue

            if message_type == "agent.turn.started":
                turn_id = str(payload.get("turn_id", ""))
                if turn_id:
                    mark_turn_started(turn_id=turn_id, node_id=node_id)
                update_node(
                    node_id,
                    {
                        "status": connection_status(node_id),
                        "running_turns": running_turn_count(payload),
                        "worker_count": payload.get("worker_count", running_turn_count(payload)),
                    },
                )
                continue

            if message_type == "agent.turn.completed":
                turn_id = str(payload.get("turn_id", ""))
                output = str(payload.get("output", "")).strip()
                if turn_id and output:
                    mark_turn_completed(turn_id=turn_id, node_id=node_id, output=output)
                update_node(
                    node_id,
                    {
                        "status": connection_status(node_id),
                        "running_turns": running_turn_count(payload),
                        "worker_count": payload.get("worker_count", 0),
                    },
                )
                continue

            send_json(
                ws,
                envelope(
                    "server.notice",
                    {"level": "info", "message": f"server ignored message type: {message_type}"},
                    source_kind="server",
                    source_id="main",
                    target_kind="agent",
                    target_id=node_id,
                    request_id=message.get("request_id"),
                ),
            )
    except Exception:
        agent_connections.remove(node_id)
        disconnected_status = "offline" if (state.get_node(node_id) or {}).get("approved", False) else "pending"
        update_node(node_id, {"status": disconnected_status})

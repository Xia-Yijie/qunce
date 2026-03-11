from __future__ import annotations

import json
import os
from datetime import UTC, datetime
from pathlib import Path
from typing import Any
from uuid import uuid4

from flask import Flask, abort, jsonify, request, send_from_directory
from flask_sock import Sock

from server.app.config import settings
from server.app.state import state

BASE_DIR = Path(__file__).resolve().parents[2]
DIST_DIR = BASE_DIR / "console" / "dist"

app = Flask(
    __name__,
    static_folder=str(DIST_DIR) if DIST_DIR.exists() else None,
    static_url_path="/",
)
sock = Sock(app)


def envelope(
    message_type: str,
    data: dict[str, Any],
    *,
    source_kind: str,
    source_id: str,
    target_kind: str,
    target_id: str,
    request_id: str | None = None,
) -> dict[str, Any]:
    return {
        "v": 1,
        "type": message_type,
        "event_id": f"evt_{uuid4().hex}",
        "request_id": request_id,
        "ts": datetime.now(tz=UTC).isoformat(),
        "source": {"kind": source_kind, "id": source_id},
        "target": {"kind": target_kind, "id": target_id},
        "data": data,
    }


def receive_json(ws: Any) -> dict[str, Any]:
    raw_message = ws.receive()
    if raw_message is None:
        raise ConnectionError("websocket closed")
    return json.loads(raw_message)


def send_json(ws: Any, payload: dict[str, Any]) -> None:
    ws.send(json.dumps(payload, ensure_ascii=False))


@app.get("/api/health")
def health() -> Any:
    return jsonify({"status": "ok"})


@app.get("/api/meta")
def meta() -> Any:
    return jsonify(
        {
            "name": settings.app_name,
            "version": settings.app_version,
            "default_pair_token": settings.default_pair_token,
        }
    )


@app.get("/api/rooms")
def rooms() -> Any:
    return jsonify(state.list_rooms())


@app.get("/api/rooms/<room_id>/snapshot")
def room_snapshot(room_id: str) -> Any:
    snapshot = state.room_snapshot(room_id)
    if snapshot is None:
        abort(404, "room_not_found")
    return jsonify(snapshot)


@app.get("/api/personas")
def personas() -> Any:
    return jsonify(state.list_personas())


@app.get("/api/nodes")
def nodes() -> Any:
    return jsonify(state.list_nodes())


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

        hostname = hello_message.get("data", {}).get("hostname", "local-node")
        node_id = auth_message.get("data", {}).get("node_id") or f"node_{hostname}"
        state.upsert_node(
            node_id,
            {
                "node_id": node_id,
                "name": hostname,
                "hostname": hostname,
                "platform": hello_message.get("data", {}).get("platform"),
                "arch": hello_message.get("data", {}).get("arch"),
                "agent_version": hello_message.get("data", {}).get("agent_version"),
                "status": "online",
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
                    "node_name": hostname,
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
                state.upsert_node(
                    node_id,
                    {
                        "status": payload.get("status", "online"),
                        "running_turns": len(payload.get("running_turns", [])),
                        "worker_count": len(payload.get("running_turns", [])),
                    },
                )
                continue

            if message_type == "agent.ping":
                state.upsert_node(
                    node_id,
                    {
                        "status": "online",
                        "running_turns": payload.get("running_turns", 0),
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
    except ConnectionError:
        state.upsert_node(node_id, {"status": "offline"})


@sock.route("/ws/console")
def console_socket(ws: Any) -> None:
    try:
        while True:
            message = receive_json(ws)
            if message.get("type") != "console.subscribe":
                send_json(
                    ws,
                    envelope(
                        "server.notice",
                        {"level": "warning", "message": "expected console.subscribe"},
                        source_kind="server",
                        source_id="main",
                        target_kind="console",
                        target_id="browser",
                        request_id=message.get("request_id"),
                    ),
                )
                continue

            room_ids = message.get("data", {}).get("room_ids", ["room_lobby"])
            for room_id in room_ids:
                snapshot = state.room_snapshot(room_id)
                if snapshot is None:
                    continue
                send_json(
                    ws,
                    envelope(
                        "server.room.snapshot",
                        snapshot,
                        source_kind="server",
                        source_id="main",
                        target_kind="console",
                        target_id="browser",
                        request_id=message.get("request_id"),
                    ),
                )

            send_json(
                ws,
                envelope(
                    "server.node.updated",
                    {"nodes": state.list_nodes()},
                    source_kind="server",
                    source_id="main",
                    target_kind="console",
                    target_id="browser",
                    request_id=message.get("request_id"),
                ),
            )
    except ConnectionError:
        return


@app.get("/")
def index() -> Any:
    if DIST_DIR.exists():
        return send_from_directory(DIST_DIR, "index.html")
    return jsonify({"message": "console build not found, run `pixi run console-build` first"})


@app.get("/<path:path>")
def spa(path: str) -> Any:
    if DIST_DIR.exists():
        asset_path = DIST_DIR / path
        if asset_path.exists() and asset_path.is_file():
            return send_from_directory(DIST_DIR, path)
        return send_from_directory(DIST_DIR, "index.html")
    abort(404)


def main() -> None:
    host = os.getenv("QUNCE_SERVER_HOST", "0.0.0.0")
    port = int(os.getenv("QUNCE_SERVER_PORT", "8000"))
    app.run(host=host, port=port, debug=False)


if __name__ == "__main__":
    main()

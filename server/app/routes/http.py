from __future__ import annotations

from typing import Any

from flask import abort, jsonify, request, send_from_directory

from server.app.config import settings
from server.app.runtime import DIST_DIR, app
from server.app.serializers import meta_payload, node_summary_payload, room_snapshot_payload, room_summary_payload
from server.app.state import state
from server.app.services.agents import agent_connections
from server.app.services.events import update_node
from server.app.services.turns import create_user_turn


@app.get("/api/health")
def health() -> Any:
    return jsonify({"status": "ok"})


@app.get("/api/meta")
def meta() -> Any:
    return jsonify(meta_payload(app_name=settings.app_name, app_version=settings.app_version))


@app.get("/api/rooms")
def rooms() -> Any:
    return jsonify([room_summary_payload(room) for room in state.rooms.values()])


@app.get("/api/rooms/<room_id>/snapshot")
def room_snapshot(room_id: str) -> Any:
    snapshot = state.room_snapshot(room_id)
    if snapshot is None:
        abort(404, "room_not_found")
    return jsonify(room_snapshot_payload(snapshot))


@app.post("/api/rooms/<room_id>/messages")
def create_room_message(room_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    content = str(payload.get("content", "")).strip()
    sender_name = str(payload.get("sender_name", "你")).strip() or "你"
    if not content:
        abort(400, "message_content_required")

    try:
        turn = create_user_turn(room_id=room_id, content=content, sender_name=sender_name)
    except ValueError:
        abort(404, "room_not_found")

    return jsonify({"ok": True, "turn": turn}), 202


@app.get("/api/personas")
def personas() -> Any:
    return jsonify(state.list_personas())


@app.get("/api/nodes")
def nodes() -> Any:
    return jsonify([node_summary_payload(node) for node in state.list_nodes()])


@app.post("/api/nodes/<node_id>/accept")
def accept_node(node_id: str) -> Any:
    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")

    payload = request.get_json(silent=True) or {}
    display_symbol = str(payload.get("display_symbol", "")).strip()[:1]
    remark = str(payload.get("remark", "")).strip()
    status = "online" if agent_connections.get(node_id) is not None else "offline"
    accepted = update_node(
        node_id,
        {
            "approved": True,
            "status": status,
            "display_symbol": display_symbol,
            "remark": remark,
        },
    )
    return jsonify(node_summary_payload(accepted))


@app.delete("/api/nodes/<node_id>")
def delete_node(node_id: str) -> Any:
    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")

    state.delete_node(node_id)
    return jsonify({"ok": True})


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

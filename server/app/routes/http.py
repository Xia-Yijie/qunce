from __future__ import annotations

import os
from pathlib import Path
from typing import Any

from flask import abort, jsonify, request, send_from_directory

from server.app.config import settings
from server.app.runtime import DIST_DIR, app
from server.app.serializers import (
    chat_snapshot_payload,
    chat_summary_payload,
    meta_payload,
    node_summary_payload,
    persona_summary_payload,
)
from server.app.state import state
from server.app.services.agents import agent_connections
from server.app.services.events import broadcast_node_updates, update_node
from server.app.services.turns import create_user_turn


@app.get("/api/health")
def health() -> Any:
    return jsonify({"status": "ok"})


@app.get("/api/meta")
def meta() -> Any:
    return jsonify(meta_payload(app_name=settings.app_name, app_version=settings.app_version))


@app.get("/api/chats")
def chats() -> Any:
    return jsonify([chat_summary_payload(chat) for chat in state.list_chats()])


@app.get("/api/chats/<chat_id>/snapshot")
def chat_snapshot(chat_id: str) -> Any:
    snapshot = state.chat_snapshot(chat_id)
    if snapshot is None:
        abort(404, "chat_not_found")
    return jsonify(chat_snapshot_payload(snapshot))


@app.post("/api/chats/<chat_id>/messages")
def create_chat_message(chat_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    content = str(payload.get("content", "")).strip()
    sender_name = str(payload.get("sender_name", "你")).strip() or "你"
    if not content:
        abort(400, "message_content_required")

    try:
        turn = create_user_turn(chat_id=chat_id, content=content, sender_name=sender_name)
    except ValueError:
        abort(404, "chat_not_found")

    return jsonify({"ok": True, "turn": turn}), 202


@app.get("/api/personas")
def personas() -> Any:
    return jsonify([persona_summary_payload(persona) for persona in state.list_personas()])


@app.post("/api/personas")
def create_persona() -> Any:
    payload = request.get_json(silent=True) or {}
    name = str(payload.get("name", "")).strip()
    node_id = str(payload.get("node_id", "")).strip()
    workspace_dir = str(payload.get("workspace_dir", "")).strip()
    role_summary = str(payload.get("role_summary", "")).strip()
    system_prompt = str(payload.get("system_prompt", "")).strip()
    agent_key = str(payload.get("agent_key", "")).strip()
    agent_label = str(payload.get("agent_label", "")).strip()

    if not name:
        abort(400, "persona_name_required")
    if not node_id:
        abort(400, "node_id_required")
    if not workspace_dir:
        abort(400, "workspace_dir_required")
    if not role_summary:
        abort(400, "role_summary_required")
    if not agent_key:
        abort(400, "agent_key_required")

    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")

    persona = state.add_persona(
        {
            "name": name,
            "node_id": node_id,
            "node_name": node.get("remark") or node.get("hostname") or node.get("name") or node_id,
            "workspace_dir": workspace_dir,
            "role_summary": role_summary,
            "system_prompt": system_prompt,
            "agent_key": agent_key,
            "agent_label": agent_label or agent_key,
            "model_provider": "codex",
        }
    )
    return jsonify(persona_summary_payload(persona)), 201


@app.get("/api/nodes")
def nodes() -> Any:
    return jsonify([node_summary_payload(node) for node in state.list_nodes()])


@app.post("/api/nodes/<node_id>/workspace-check")
def validate_node_workspace(node_id: str) -> Any:
    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")

    payload = request.get_json(silent=True) or {}
    raw_workspace_dir = str(payload.get("workspace_dir", "")).strip()
    if not raw_workspace_dir:
        abort(400, "workspace_dir_required")

    workspace_dir = Path(raw_workspace_dir).expanduser()
    if not workspace_dir.is_absolute():
        return jsonify(
            {
                "ok": False,
                "normalized_path": str(workspace_dir),
                "message": "工作目录必须是绝对路径",
            }
        ), 200

    if workspace_dir.exists():
        if not workspace_dir.is_dir():
            return jsonify(
                {
                    "ok": False,
                    "normalized_path": str(workspace_dir),
                    "message": "该路径已存在，但不是目录",
                }
            ), 200
        if not os.access(workspace_dir, os.R_OK | os.W_OK | os.X_OK):
            return jsonify(
                {
                    "ok": False,
                    "normalized_path": str(workspace_dir),
                    "message": "该目录当前不可读写",
                }
            ), 200
        if any(workspace_dir.iterdir()):
            return jsonify(
                {
                    "ok": False,
                    "normalized_path": str(workspace_dir),
                    "message": "该目录不是空目录",
                }
            ), 200
        return jsonify(
            {
                "ok": True,
                "normalized_path": str(workspace_dir),
                "message": "目录可用：已存在且为空目录",
            }
        )

    parent_dir = workspace_dir.parent
    if not parent_dir.exists():
        return jsonify(
            {
                "ok": False,
                "normalized_path": str(workspace_dir),
                "message": "父目录不存在，暂时不能在这里创建",
            }
        ), 200
    if not parent_dir.is_dir():
        return jsonify(
            {
                "ok": False,
                "normalized_path": str(workspace_dir),
                "message": "父路径不是目录",
            }
        ), 200
    if not os.access(parent_dir, os.W_OK | os.X_OK):
        return jsonify(
            {
                "ok": False,
                "normalized_path": str(workspace_dir),
                "message": "父目录没有创建权限",
            }
        ), 200

    return jsonify(
        {
            "ok": True,
            "normalized_path": str(workspace_dir),
            "message": "目录可用：目标目录不存在，但可以创建",
        }
    )


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

    agent_connections.disconnect(node_id)
    state.delete_node(node_id)
    broadcast_node_updates()
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

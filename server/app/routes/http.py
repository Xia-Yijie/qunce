from __future__ import annotations

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
from server.app.services.agents import agent_connections
from server.app.services.events import add_chat_message, broadcast_chat_snapshot, broadcast_node_updates, update_node
from server.app.services.turns import create_user_turn, set_chat_member_muted, set_chat_muted
from server.app.state import state


@app.get("/api/health")
def health() -> Any:
    return jsonify({"status": "ok"})


@app.get("/api/meta")
def meta() -> Any:
    payload = meta_payload(app_name=settings.app_name, app_version=settings.app_version)
    payload["routes"] = sorted(
        (rule.rule, sorted(rule.methods - {"HEAD", "OPTIONS"}))
        for rule in app.url_map.iter_rules()
        if rule.rule.startswith("/api/")
    )
    return jsonify(payload)


@app.get("/api/chats")
def chats() -> Any:
    return jsonify([chat_summary_payload(chat) for chat in state.list_chats()])


@app.post("/api/chats")
def create_chat() -> Any:
    payload = request.get_json(silent=True) or {}
    raw_persona_ids = payload.get("persona_ids")
    name = str(payload.get("name", "")).strip()
    persona_ids: list[str] = []
    if isinstance(raw_persona_ids, list):
        persona_ids = [str(item).strip() for item in raw_persona_ids if str(item).strip()]
    else:
        persona_id = str(payload.get("persona_id", "")).strip()
        if persona_id:
            persona_ids = [persona_id]
    if not persona_ids:
        abort(400, "persona_id_required")

    personas = [persona for persona in state.list_personas() if str(persona.get("persona_id", "")) in set(persona_ids)]
    if len(personas) != len(set(persona_ids)):
        abort(404, "persona_not_found")

    ordered_personas = sorted(personas, key=lambda persona: persona_ids.index(str(persona.get("persona_id", ""))))
    chat = state.create_or_get_chat(ordered_personas, name=name)
    return jsonify(chat_summary_payload(chat)), 201


@app.get("/api/chats/<chat_id>/snapshot")
def chat_snapshot(chat_id: str) -> Any:
    snapshot = state.chat_snapshot(chat_id)
    if snapshot is None:
        abort(404, "chat_not_found")
    return jsonify(chat_snapshot_payload(snapshot))


@app.post("/api/chats/<chat_id>/members")
def add_chat_members(chat_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    raw_persona_ids = payload.get("persona_ids")
    actor_name = str(payload.get("actor_name", "群成员")).strip() or "群成员"
    if not isinstance(raw_persona_ids, list):
        abort(400, "persona_ids_required")

    persona_ids = [str(item).strip() for item in raw_persona_ids if str(item).strip()]
    if not persona_ids:
        abort(400, "persona_ids_required")

    chat = state.chat_snapshot(chat_id)
    if chat is None:
        abort(404, "chat_not_found")

    personas = [persona for persona in state.list_personas() if str(persona.get("persona_id", "")) in set(persona_ids)]
    if len(personas) != len(set(persona_ids)):
        abort(404, "persona_not_found")

    ordered_personas = sorted(personas, key=lambda persona: persona_ids.index(str(persona.get("persona_id", ""))))
    result = state.add_members_to_chat(chat_id, ordered_personas)
    if result is None:
        abort(404, "chat_not_found")

    snapshot = result["chat"]
    added_persona_ids = list(result.get("added_persona_ids", []))
    if added_persona_ids:
        added_names = [str(persona.get("name", "")).strip() for persona in ordered_personas if str(persona.get("persona_id", "")) in set(added_persona_ids)]
        add_chat_message(
            chat_id,
            sender_type="event",
            sender_name=actor_name,
            content=f"已添加成员：{'、'.join(name for name in added_names if name)}",
            metadata={"event_type": "chat.members.added", "persona_ids": added_persona_ids},
        )
        snapshot = state.chat_snapshot(chat_id) or snapshot
    else:
        broadcast_chat_snapshot(chat_id)

    return jsonify(
        {
            "chat": chat_snapshot_payload(snapshot),
            "added_persona_ids": added_persona_ids,
        }
    ), 200


@app.delete("/api/chats/<chat_id>/members/<persona_id>")
def remove_chat_member(chat_id: str, persona_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    actor_name = str(payload.get("actor_name", "群成员")).strip() or "群成员"

    result = state.remove_member_from_chat(chat_id, persona_id)
    if result is None:
        abort(404, "chat_not_found")

    removed_member = result.get("removed_member")
    if not result.get("removed", False):
        abort(404, "persona_not_in_chat")

    dissolved = bool(result.get("dissolved", False))
    snapshot = result.get("chat")
    removed_name = str((removed_member or {}).get("name", "")).strip() or persona_id
    if dissolved:
        return jsonify({"ok": True, "dissolved": True, "removed_persona_id": persona_id, "removed_persona_name": removed_name}), 200

    add_chat_message(
        chat_id,
        sender_type="event",
        sender_name=actor_name,
        content=f"已移出成员：{removed_name}",
        metadata={"event_type": "chat.member.removed", "persona_id": persona_id},
    )
    latest_snapshot = state.chat_snapshot(chat_id) or snapshot
    return jsonify(
        {
            "ok": True,
            "dissolved": False,
            "removed_persona_id": persona_id,
            "removed_persona_name": removed_name,
            "chat": chat_snapshot_payload(latest_snapshot),
        }
    ), 200


@app.delete("/api/chats/<chat_id>")
def delete_chat(chat_id: str) -> Any:
    snapshot = state.delete_chat(chat_id)
    if snapshot is None:
        abort(404, "chat_not_found")
    return jsonify({"ok": True, "dissolved": True, "chat_id": chat_id, "member_count": len(snapshot.get("members", []))}), 200


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
    except PermissionError:
        return jsonify({"ok": False, "message": "当前群聊已开启全体禁言"}), 409

    return jsonify({"ok": True, "turn": turn}), 202


@app.post("/api/chats/<chat_id>/mute")
def toggle_chat_mute(chat_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    muted = bool(payload.get("muted", False))
    actor_name = str(payload.get("actor_name", "群状态")).strip() or "群状态"

    try:
        snapshot = set_chat_muted(chat_id=chat_id, muted=muted, actor_name=actor_name)
    except ValueError:
        abort(404, "chat_not_found")

    return jsonify(chat_snapshot_payload(snapshot)), 200


@app.post("/api/chats/<chat_id>/members/<persona_id>/mute")
def toggle_chat_member_mute(chat_id: str, persona_id: str) -> Any:
    payload = request.get_json(silent=True) or {}
    muted = bool(payload.get("muted", False))
    actor_name = str(payload.get("actor_name", "群成员")).strip() or "群成员"

    try:
        snapshot = set_chat_member_muted(
            chat_id=chat_id,
            persona_id=persona_id,
            muted=muted,
            actor_name=actor_name,
        )
    except ValueError:
        abort(404, "chat_not_found")
    except LookupError:
        abort(404, "persona_not_in_chat")

    return jsonify(chat_snapshot_payload(snapshot)), 200


@app.get("/api/personas")
def personas() -> Any:
    return jsonify([persona_summary_payload(persona) for persona in state.list_personas()])


@app.post("/api/personas")
def create_persona() -> Any:
    payload = request.get_json(silent=True) or {}
    action = str(payload.get("_action", "")).strip()

    if action == "delete":
        persona_id = str(payload.get("persona_id", "")).strip()
        if not persona_id:
            abort(400, "persona_id_required")
        persona = state.delete_persona(persona_id)
        if persona is None:
            abort(404, "persona_not_found")
        return jsonify({"ok": True})

    if action == "create_chat":
        raw_persona_ids = payload.get("persona_ids")
        name = str(payload.get("name", "")).strip()
        persona_ids: list[str] = []
        if isinstance(raw_persona_ids, list):
            persona_ids = [str(item).strip() for item in raw_persona_ids if str(item).strip()]
        if not persona_ids:
            abort(400, "persona_id_required")

        personas = [persona for persona in state.list_personas() if str(persona.get("persona_id", "")) in set(persona_ids)]
        if len(personas) != len(set(persona_ids)):
            abort(404, "persona_not_found")

        ordered_personas = sorted(personas, key=lambda persona: persona_ids.index(str(persona.get("persona_id", ""))))
        chat = state.create_or_get_chat(ordered_personas, name=name)
        return jsonify(chat_summary_payload(chat)), 201

    name = str(payload.get("name", "")).strip()
    node_id = str(payload.get("node_id", "")).strip()
    workspace_dir = str(payload.get("workspace_dir", "")).strip()
    system_prompt = str(payload.get("system_prompt", "")).strip()
    agent_key = str(payload.get("agent_key", "")).strip()
    agent_label = str(payload.get("agent_label", "")).strip()
    avatar_symbol = str(payload.get("avatar_symbol", "")).strip()[:1]
    avatar_bg_color = str(payload.get("avatar_bg_color", "")).strip()
    avatar_text_color = str(payload.get("avatar_text_color", "")).strip()

    if not name:
        abort(400, "persona_name_required")
    if not node_id:
        abort(400, "node_id_required")
    if not workspace_dir:
        abort(400, "workspace_dir_required")
    if not system_prompt:
        abort(400, "system_prompt_required")
    if not agent_key:
        abort(400, "agent_key_required")

    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")
    if agent_connections.get(node_id) is None:
        return jsonify({"ok": False, "message": "客户端不在线，不能创建"}), 409

    try:
        workspace_check = agent_connections.request_workspace_validation(node_id=node_id, workspace_dir=workspace_dir)
    except TimeoutError:
        return jsonify({"ok": False, "message": "客户端校验超时，请确认节点在线后重试"}), 409
    except ConnectionError:
        return jsonify({"ok": False, "message": "客户端不在线，不能创建"}), 409

    if not workspace_check.get("ok", False):
        return jsonify({"ok": False, "message": workspace_check.get("message", "工作目录不可用")}), 400

    persona = state.add_persona(
        {
            "name": name,
            "node_id": node_id,
            "node_name": node.get("remark") or node.get("hostname") or node.get("name") or node_id,
            "workspace_dir": str(workspace_check.get("normalized_path") or workspace_dir),
            "system_prompt": system_prompt,
            "agent_key": agent_key,
            "agent_label": agent_label or agent_key,
            "model_provider": "codex",
            "avatar_symbol": avatar_symbol or name[:1],
            "avatar_bg_color": avatar_bg_color or "#d9e6f8",
            "avatar_text_color": avatar_text_color or "#31547e",
        }
    )
    return jsonify(persona_summary_payload(persona)), 201


@app.get("/api/nodes")
def nodes() -> Any:
    payload = []
    for node in state.list_nodes():
        if node.get("approved", False):
            node = {
                **node,
                "status": "online" if agent_connections.get(str(node.get("node_id", ""))) is not None else "offline",
            }
        payload.append(node_summary_payload(node))
    return jsonify(payload)


@app.post("/api/nodes/<node_id>/workspace-check")
def validate_node_workspace(node_id: str) -> Any:
    node = state.get_node(node_id)
    if node is None:
        abort(404, "node_not_found")

    payload = request.get_json(silent=True) or {}
    workspace_dir = str(payload.get("workspace_dir", "")).strip()
    if not workspace_dir:
        abort(400, "workspace_dir_required")

    if agent_connections.get(node_id) is None:
        return jsonify(
            {
                "ok": False,
                "normalized_path": workspace_dir,
                "message": "客户端不在线，不能创建",
            }
        ), 200

    try:
        result = agent_connections.request_workspace_validation(node_id=node_id, workspace_dir=workspace_dir)
    except TimeoutError:
        return jsonify(
            {
                "ok": False,
                "normalized_path": workspace_dir,
                "message": "客户端校验超时，请确认节点在线后重试",
            }
        ), 200
    except ConnectionError:
        return jsonify(
            {
                "ok": False,
                "normalized_path": workspace_dir,
                "message": "客户端不在线，不能创建",
            }
        ), 200

    return jsonify(result), 200


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

from __future__ import annotations

from copy import deepcopy
from typing import Any


def meta_payload(*, app_name: str, app_version: str) -> dict[str, Any]:
    return {
        "name": app_name,
        "version": app_version,
    }


def chat_summary_payload(chat: dict[str, Any]) -> dict[str, Any]:
    visible_messages = [message for message in chat.get("messages", []) if message.get("sender_type") != "system"]
    last_message_at = None
    last_message_preview = None
    if visible_messages:
        last_message_at = visible_messages[-1].get("created_at")
        last_message_preview = str(visible_messages[-1].get("content", "")).strip() or None
    return {
        "chat_id": chat["chat_id"],
        "name": chat["name"],
        "mode": chat["mode"],
        "muted": bool(chat.get("muted", False)),
        "pinned": bool(chat.get("pinned", False)),
        "dnd": bool(chat.get("dnd", False)),
        "marked_unread": bool(chat.get("marked_unread", False)),
        "unread_count": int(chat.get("unread_count", 0)),
        "member_count": len(chat.get("members", [])),
        "message_count": len(visible_messages),
        "last_message_at": last_message_at,
        "last_message_preview": last_message_preview,
    }


def chat_snapshot_payload(chat: dict[str, Any]) -> dict[str, Any]:
    payload = deepcopy(chat)
    members = [
        {
            "persona_id": str(member.get("persona_id", "")),
            "name": str(member.get("name", "")),
            "status": str(member.get("status", "active")),
            "muted": bool(member.get("muted", False)),
        }
        for member in payload.get("members", [])
        if str(member.get("persona_id", "")).strip()
    ]
    turns_by_message_id: dict[str, list[dict[str, Any]]] = {}
    for turn in payload.get("turns", []):
        if not isinstance(turn, dict):
            continue
        message_id = str(turn.get("message_id", "")).strip()
        if not message_id:
            continue
        turns_by_message_id.setdefault(message_id, []).append(turn)

    messages: list[dict[str, Any]] = []
    for message in payload.get("messages", []):
        if message.get("sender_type") == "system":
            continue

        normalized = {
            **message,
            "metadata": dict(message.get("metadata") or {}),
        }
        if normalized.get("sender_type") == "user":
            message_turns = turns_by_message_id.get(str(normalized.get("message_id", "")), [])
            read_persona_ids = {
                str(turn.get("persona_id", "")).strip()
                for turn in message_turns
                if str(turn.get("status", "")) in {"read", "running", "completed"}
            }
            readable_members = [member for member in members if not member["muted"]]
            read_by = deepcopy([member for member in readable_members if member["persona_id"] in read_persona_ids])
            unread_by = deepcopy([member for member in readable_members if member["persona_id"] not in read_persona_ids])
            normalized["read_receipt"] = {
                "read_count": len(read_by),
                "unread_count": len(unread_by),
                "total_count": len(readable_members),
                "read_by": read_by,
                "unread_by": unread_by,
            }
        messages.append(normalized)

    payload["messages"] = messages
    payload["muted"] = bool(payload.get("muted", False))
    payload["pinned"] = bool(payload.get("pinned", False))
    payload["dnd"] = bool(payload.get("dnd", False))
    payload["marked_unread"] = bool(payload.get("marked_unread", False))
    payload["unread_count"] = int(payload.get("unread_count", 0))
    return payload


def persona_summary_payload(persona: dict[str, Any]) -> dict[str, Any]:
    return {
        "persona_id": persona["persona_id"],
        "name": persona["name"],
        "status": persona.get("status", "active"),
        "node_id": persona.get("node_id", ""),
        "node_name": persona.get("node_name", ""),
        "workspace_dir": persona.get("workspace_dir", ""),
        "system_prompt": persona.get("system_prompt", ""),
        "agent_key": persona.get("agent_key", "codex-general"),
        "agent_label": persona.get("agent_label", "codex"),
        "model_provider": persona.get("model_provider", "codex"),
        "avatar_symbol": persona.get("avatar_symbol", ""),
        "avatar_bg_color": persona.get("avatar_bg_color", ""),
        "avatar_text_color": persona.get("avatar_text_color", ""),
    }


def node_summary_payload(node: dict[str, Any]) -> dict[str, Any]:
    status = str(node.get("status", "offline"))
    status_label = {
        "pending": "待接受",
        "online": "在线",
        "offline": "离线",
    }.get(status, status)
    approved = bool(node.get("approved", False))
    return {
        "node_id": node.get("node_id"),
        "name": node.get("name"),
        "hostname": node.get("hostname", ""),
        "display_symbol": node.get("display_symbol", ""),
        "remark": node.get("remark", ""),
        "status": status,
        "status_label": status_label,
        "approved": approved,
        "can_accept": not approved,
        "hello_message": node.get("hello_message", ""),
        "work_dir": node.get("work_dir", ""),
        "platform": node.get("platform"),
        "arch": node.get("arch"),
        "last_seen_at": node.get("last_seen_at"),
        "running_turns": node.get("running_turns", 0),
        "worker_count": node.get("worker_count", 0),
    }


def node_list_payload(nodes: list[dict[str, Any]]) -> dict[str, Any]:
    return {"nodes": [node_summary_payload(node) for node in nodes]}

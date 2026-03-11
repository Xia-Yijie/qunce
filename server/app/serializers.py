from __future__ import annotations

from copy import deepcopy
from typing import Any


def meta_payload(*, app_name: str, app_version: str) -> dict[str, Any]:
    return {
        "name": app_name,
        "version": app_version,
    }


def chat_summary_payload(chat: dict[str, Any]) -> dict[str, Any]:
    return {
        "chat_id": chat["chat_id"],
        "name": chat["name"],
        "mode": chat["mode"],
        "member_count": len(chat.get("members", [])),
        "message_count": len(chat.get("messages", [])),
    }


def chat_snapshot_payload(chat: dict[str, Any]) -> dict[str, Any]:
    return deepcopy(chat)


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
        "platform": node.get("platform"),
        "arch": node.get("arch"),
        "last_seen_at": node.get("last_seen_at"),
    }


def node_list_payload(nodes: list[dict[str, Any]]) -> dict[str, Any]:
    return {"nodes": [node_summary_payload(node) for node in nodes]}

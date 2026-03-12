from __future__ import annotations

from typing import Any

from server.app.services.agents import dispatch_turn_to_agent
from server.app.services.events import add_chat_message
from server.app.state import state


def create_user_turn(*, chat_id: str, content: str, sender_name: str = "你") -> dict[str, Any]:
    chat = state.chat_snapshot(chat_id)
    if chat is None:
        raise ValueError("chat_not_found")
    if chat.get("mode") == "group" and bool(chat.get("muted", False)):
        raise PermissionError("chat_muted")

    snapshot = add_chat_message(
        chat_id,
        sender_type="user",
        sender_name=sender_name,
        content=content,
    )
    if snapshot is None:
        raise ValueError("chat_not_found")

    message = snapshot.get("messages", [])[-1]
    message_id = str(message.get("message_id", ""))
    members = list(snapshot.get("members", []))
    muted = bool(snapshot.get("muted", False))
    personas_by_id = {
        str(persona.get("persona_id", "")): persona
        for persona in state.list_personas()
        if str(persona.get("persona_id", "")).strip()
    }

    turns: list[dict[str, Any]] = []
    dispatched = 0
    for member in members:
        persona_id = str(member.get("persona_id", "")).strip()
        persona = personas_by_id.get(persona_id)
        if persona is None:
            continue

        node_id = str(persona.get("node_id", "")).strip() or None
        turn = state.create_turn(
            chat_id,
            content=content,
            node_id=node_id,
            sender_name=sender_name,
            status="pending",
            extra={
                "persona_id": persona_id,
                "persona_name": str(persona.get("name", "")).strip() or persona_id,
                "message_id": message_id,
                "workspace_dir": str(persona.get("workspace_dir", "")).strip(),
                "system_prompt": str(persona.get("system_prompt", "")).strip(),
                "agent_key": str(persona.get("agent_key", "")).strip(),
                "agent_label": str(persona.get("agent_label", "")).strip(),
                "muted": muted,
            },
        )
        if turn is None:
            continue
        turns.append(turn)

        if not node_id:
            continue

        try:
            dispatch_turn_to_agent(node_id=node_id, turn=turn)
            dispatched += 1
        except ConnectionError:
            state.update_turn(turn["turn_id"], {"status": "pending", "assigned_node_id": node_id})

    return {
        "message_id": message_id,
        "turn_count": len(turns),
        "dispatched_count": dispatched,
    }


def mark_turn_read(*, turn_id: str, node_id: str) -> dict[str, Any] | None:
    return state.update_turn(turn_id, {"status": "read", "assigned_node_id": node_id})


def mark_turn_started(*, turn_id: str, node_id: str) -> dict[str, Any] | None:
    return state.update_turn(turn_id, {"status": "running", "assigned_node_id": node_id})


def mark_turn_completed(*, turn_id: str, node_id: str, output: str) -> dict[str, Any] | None:
    turn = state.update_turn(turn_id, {"status": "completed", "assigned_node_id": node_id, "output": output})
    if turn is not None:
        add_chat_message(
            turn["chat_id"],
            sender_type="agent",
            sender_name=str(turn.get("persona_name", "")).strip() or f"节点 {node_id}",
            content=output,
            metadata={
                "turn_id": turn_id,
                "node_id": node_id,
                "persona_id": turn.get("persona_id"),
            },
        )
    return turn


def dispatch_pending_turns_for_node(node_id: str) -> int:
    dispatched = 0
    for turn in state.list_pending_turns_for_node(node_id):
        try:
            dispatch_turn_to_agent(node_id=node_id, turn=turn)
            dispatched += 1
        except ConnectionError:
            break
    return dispatched


def set_chat_muted(*, chat_id: str, muted: bool, actor_name: str = "群状态") -> dict[str, Any]:
    chat = state.update_chat(chat_id, {"muted": muted})
    if chat is None:
        raise ValueError("chat_not_found")

    snapshot = add_chat_message(
        chat_id,
        sender_type="event",
        sender_name=actor_name,
        content="已开启全体禁言" if muted else "已关闭全体禁言",
        metadata={"event_type": "chat.muted", "muted": muted},
    )
    if snapshot is None:
        raise ValueError("chat_not_found")
    return snapshot

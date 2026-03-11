from __future__ import annotations

from typing import Any

from server.app.services.agents import dispatch_turn_to_agent
from server.app.services.events import add_room_message
from server.app.state import state


def choose_online_node() -> str | None:
    for node in state.list_nodes():
        if node.get("status") == "online":
            return str(node["node_id"])
    return None


def create_user_turn(*, room_id: str, content: str, sender_name: str = "你") -> dict[str, Any]:
    add_room_message(
        room_id,
        sender_type="user",
        sender_name=sender_name,
        content=content,
    )

    node_id = choose_online_node()
    turn = state.create_turn(room_id, content=content, node_id=node_id, sender_name=sender_name)
    if turn is None:
        raise ValueError("room_not_found")

    if node_id is None:
        state.update_turn(turn["turn_id"], {"status": "queued"})
        add_room_message(
            room_id,
            sender_type="system",
            sender_name="系统",
            content="当前没有在线节点，任务已创建但还未派发。",
            metadata={"turn_id": turn["turn_id"]},
        )
        return turn

    try:
        dispatch_turn_to_agent(node_id=node_id, turn=turn)
    except ConnectionError:
        state.update_turn(turn["turn_id"], {"status": "queued", "assigned_node_id": None})
        add_room_message(
            room_id,
            sender_type="system",
            sender_name="系统",
            content="节点连接刚刚断开，任务已回退为等待派发。",
            metadata={"turn_id": turn["turn_id"]},
        )
        return state.get_turn(turn["turn_id"]) or turn

    state.update_turn(turn["turn_id"], {"status": "assigned"})
    add_room_message(
        room_id,
        sender_type="system",
        sender_name="系统",
        content=f"已创建任务并派发给节点 {node_id}。",
        metadata={"turn_id": turn["turn_id"], "node_id": node_id},
    )
    return state.get_turn(turn["turn_id"]) or turn


def mark_turn_started(*, turn_id: str, node_id: str) -> dict[str, Any] | None:
    turn = state.update_turn(turn_id, {"status": "running", "assigned_node_id": node_id})
    if turn is not None:
        add_room_message(
            turn["room_id"],
            sender_type="system",
            sender_name="系统",
            content=f"节点 {node_id} 正在处理这轮任务。",
            metadata={"turn_id": turn_id, "node_id": node_id},
        )
    return turn


def mark_turn_completed(*, turn_id: str, node_id: str, output: str) -> dict[str, Any] | None:
    turn = state.update_turn(turn_id, {"status": "completed", "assigned_node_id": node_id, "output": output})
    if turn is not None:
        add_room_message(
            turn["room_id"],
            sender_type="agent",
            sender_name=f"节点 {node_id}",
            content=output,
            metadata={"turn_id": turn_id, "node_id": node_id},
        )
    return turn

from __future__ import annotations

from threading import Lock
from typing import Any

from server.app.protocol import envelope, send_json
from server.app.serializers import node_list_payload, room_snapshot_payload
from server.app.state import state


class ConsoleSubscriptionRegistry:
    def __init__(self) -> None:
        self._lock = Lock()
        self._subscriptions: dict[int, dict[str, Any]] = {}

    def add(self, ws: Any, room_ids: list[str], *, watch_nodes: bool) -> int:
        subscription_id = id(ws)
        with self._lock:
            self._subscriptions[subscription_id] = {
                "ws": ws,
                "send_lock": Lock(),
                "room_ids": list(dict.fromkeys(room_ids)),
                "watch_nodes": watch_nodes,
            }
        return subscription_id

    def update(self, subscription_id: int, room_ids: list[str], *, watch_nodes: bool) -> None:
        with self._lock:
            subscription = self._subscriptions.get(subscription_id)
            if subscription is None:
                return
            subscription["room_ids"] = list(dict.fromkeys(room_ids))
            subscription["watch_nodes"] = watch_nodes

    def remove(self, subscription_id: int) -> None:
        with self._lock:
            self._subscriptions.pop(subscription_id, None)

    def get(self, subscription_id: int) -> dict[str, Any] | None:
        with self._lock:
            return self._subscriptions.get(subscription_id)

    def items(self) -> list[tuple[int, dict[str, Any]]]:
        with self._lock:
            return list(self._subscriptions.items())


console_subscriptions = ConsoleSubscriptionRegistry()


def send_payload(subscription_id: int, payload: dict[str, Any]) -> None:
    subscription = console_subscriptions.get(subscription_id)
    if subscription is None:
        raise ConnectionError("subscription closed")

    with subscription["send_lock"]:
        send_json(subscription["ws"], payload)


def send_room_snapshots(subscription_id: int, request_id: str | None) -> None:
    subscription = console_subscriptions.get(subscription_id)
    if subscription is None:
        return

    for room_id in subscription["room_ids"]:
        snapshot = state.room_snapshot(room_id)
        if snapshot is None:
            continue
        send_payload(
            subscription_id,
            envelope(
                "server.room.snapshot",
                room_snapshot_payload(snapshot),
                source_kind="server",
                source_id="main",
                target_kind="console",
                target_id="browser",
                request_id=request_id,
            ),
        )


def send_node_update(subscription_id: int, request_id: str | None = None) -> None:
    send_payload(
        subscription_id,
        envelope(
            "server.node.updated",
            node_list_payload(state.list_nodes()),
            source_kind="server",
            source_id="main",
            target_kind="console",
            target_id="browser",
            request_id=request_id,
        ),
    )


def broadcast_room_snapshot(room_id: str) -> None:
    stale_subscription_ids: list[int] = []
    for subscription_id, subscription in console_subscriptions.items():
        if room_id not in subscription["room_ids"]:
            continue
        try:
            send_room_snapshots(subscription_id, None)
        except Exception:
            stale_subscription_ids.append(subscription_id)

    for subscription_id in stale_subscription_ids:
        console_subscriptions.remove(subscription_id)


def broadcast_node_updates() -> None:
    stale_subscription_ids: list[int] = []
    for subscription_id, subscription in console_subscriptions.items():
        if not subscription.get("watch_nodes", False):
            continue
        try:
            send_node_update(subscription_id)
        except Exception:
            stale_subscription_ids.append(subscription_id)

    for subscription_id in stale_subscription_ids:
        console_subscriptions.remove(subscription_id)


def update_node(node_id: str, payload: dict[str, Any]) -> dict[str, Any]:
    node = state.upsert_node(node_id, payload)
    broadcast_node_updates()
    return node


def add_room_message(
    room_id: str,
    *,
    sender_type: str,
    sender_name: str,
    content: str,
    status: str = "completed",
    metadata: dict | None = None,
) -> dict | None:
    room = state.add_room_message(
        room_id,
        sender_type=sender_type,
        sender_name=sender_name,
        content=content,
        status=status,
        metadata=metadata,
    )
    if room is not None:
        broadcast_room_snapshot(room_id)
    return room

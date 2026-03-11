from __future__ import annotations

from typing import Any

from server.app.protocol import envelope, receive_json, send_json
from server.app.runtime import sock
from server.app.services.events import console_subscriptions, send_chat_snapshots, send_node_update, send_payload


@sock.route("/ws/console")
def console_socket(ws: Any) -> None:
    subscription_id: int | None = None
    try:
        while True:
            message = receive_json(ws)
            if message.get("type") != "console.subscribe":
                payload = envelope(
                    "server.notice",
                    {"level": "warning", "message": "expected console.subscribe"},
                    source_kind="server",
                    source_id="main",
                    target_kind="console",
                    target_id="browser",
                    request_id=message.get("request_id"),
                )
                if subscription_id is None:
                    send_json(ws, payload)
                else:
                    send_payload(subscription_id, payload)
                continue

            chat_ids = message.get("data", {}).get("chat_ids", [])
            watch_nodes = bool(message.get("data", {}).get("watch_nodes", False))

            if subscription_id is None:
                subscription_id = console_subscriptions.add(
                    ws,
                    chat_ids,
                    watch_nodes=watch_nodes,
                )
            else:
                console_subscriptions.update(
                    subscription_id,
                    chat_ids,
                    watch_nodes=watch_nodes,
                )

            send_chat_snapshots(subscription_id, message.get("request_id"))
            if watch_nodes:
                send_node_update(subscription_id, message.get("request_id"))
    except ConnectionError:
        if subscription_id is not None:
            console_subscriptions.remove(subscription_id)

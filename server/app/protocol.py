from __future__ import annotations

import json
from datetime import UTC, datetime
from typing import Any
from uuid import uuid4


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

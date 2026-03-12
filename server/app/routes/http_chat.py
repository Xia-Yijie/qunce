from __future__ import annotations

from typing import Any

from flask import jsonify, request

from server.app.routes._helpers import load_ordered_personas, parse_persona_ids
from server.app.runtime import app
from server.app.serializers import chat_summary_payload
from server.app.state import state


@app.post("/api/chat-start")
def start_chat() -> Any:
    payload = request.get_json(silent=True) or {}
    raw_persona_ids = payload.get("persona_ids")
    name = str(payload.get("name", "")).strip()
    persona_ids = parse_persona_ids(raw_persona_ids, error_code="persona_id_required")
    ordered_personas = load_ordered_personas(persona_ids)
    chat = state.create_or_get_chat(ordered_personas, name=name)
    return jsonify(chat_summary_payload(chat)), 201

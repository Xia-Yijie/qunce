from __future__ import annotations

from typing import Any

from flask import abort, jsonify, request

from server.app.runtime import app
from server.app.serializers import chat_summary_payload
from server.app.state import state


@app.post("/api/chat-start")
def start_chat() -> Any:
    payload = request.get_json(silent=True) or {}
    raw_persona_ids = payload.get("persona_ids")
    if not isinstance(raw_persona_ids, list):
        abort(400, "persona_id_required")

    persona_ids = [str(item).strip() for item in raw_persona_ids if str(item).strip()]
    if not persona_ids:
        abort(400, "persona_id_required")

    personas = [persona for persona in state.list_personas() if str(persona.get("persona_id", "")) in set(persona_ids)]
    if len(personas) != len(set(persona_ids)):
        abort(404, "persona_not_found")

    ordered_personas = sorted(personas, key=lambda persona: persona_ids.index(str(persona.get("persona_id", ""))))
    chat = state.create_or_get_chat(ordered_personas)
    return jsonify(chat_summary_payload(chat)), 201

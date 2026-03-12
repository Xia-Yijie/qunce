from __future__ import annotations

from flask import abort

from server.app.state import state


def parse_persona_ids(raw_persona_ids: object, *, error_code: str = "persona_ids_required") -> list[str]:
    if not isinstance(raw_persona_ids, list):
        abort(400, error_code)

    persona_ids = [str(item).strip() for item in raw_persona_ids if str(item).strip()]
    if not persona_ids:
        abort(400, error_code)
    return persona_ids


def load_ordered_personas(persona_ids: list[str]) -> list[dict]:
    personas = [persona for persona in state.list_personas() if str(persona.get("persona_id", "")) in set(persona_ids)]
    if len(personas) != len(set(persona_ids)):
        abort(404, "persona_not_found")
    return sorted(personas, key=lambda persona: persona_ids.index(str(persona.get("persona_id", ""))))

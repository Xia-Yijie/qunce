from __future__ import annotations

import json
import sqlite3
from copy import deepcopy
from datetime import UTC, datetime
from pathlib import Path
from threading import Lock
from uuid import uuid4

from server.app.config import settings


class StateStore:
    def __init__(self, db_path: Path) -> None:
        self._lock = Lock()
        self._db_path = db_path
        self._db_path.parent.mkdir(parents=True, exist_ok=True)
        self._conn = sqlite3.connect(self._db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._init_schema()
        self._migrate_persona_payloads()
        self._migrate_chat_payloads()

    def _init_schema(self) -> None:
        with self._conn:
            self._conn.executescript(
                """
                PRAGMA journal_mode = WAL;
                PRAGMA busy_timeout = 5000;

                CREATE TABLE IF NOT EXISTS nodes (
                    node_id TEXT PRIMARY KEY,
                    payload TEXT NOT NULL,
                    last_seen_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS personas (
                    persona_id TEXT PRIMARY KEY,
                    payload TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS chats (
                    chat_id TEXT PRIMARY KEY,
                    payload TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS chat_messages (
                    message_id TEXT PRIMARY KEY,
                    chat_id TEXT NOT NULL,
                    payload TEXT NOT NULL,
                    created_at TEXT NOT NULL
                );

                CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_id_created_at
                ON chat_messages(chat_id, created_at);

                CREATE TABLE IF NOT EXISTS turns (
                    turn_id TEXT PRIMARY KEY,
                    chat_id TEXT NOT NULL,
                    payload TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE INDEX IF NOT EXISTS idx_turns_chat_id_created_at
                ON turns(chat_id, created_at);
                """
            )

    def _migrate_persona_payloads(self) -> None:
        with self._lock, self._conn:
            rows = self._conn.execute("SELECT persona_id, payload, updated_at FROM personas").fetchall()
            for row in rows:
                payload = self._load(row["payload"])
                if "role_summary" not in payload:
                    continue
                payload.pop("role_summary", None)
                self._conn.execute(
                    "UPDATE personas SET payload = ?, updated_at = ? WHERE persona_id = ?",
                    (self._dump(payload), row["updated_at"], row["persona_id"]),
                )

    def _migrate_chat_payloads(self) -> None:
        with self._lock, self._conn:
            rows = self._conn.execute("SELECT chat_id, payload, updated_at FROM chats").fetchall()
            for row in rows:
                payload = self._load(row["payload"])
                previous_mode = str(payload.get("mode", "")).strip()
                members = [member for member in payload.get("members", []) if isinstance(member, dict)]
                next_name = "、".join(
                    str(member.get("name", "")).strip() for member in members[:3] if str(member.get("name", "")).strip()
                ) or "新聊天"
                changed = False
                if previous_mode != "group":
                    payload["mode"] = "group"
                    changed = True
                if "remark" in payload:
                    payload.pop("remark", None)
                    changed = True
                for key in ("pinned", "dnd", "marked_unread"):
                    if key not in payload:
                        payload[key] = False
                        changed = True
                if "unread_count" not in payload:
                    payload["unread_count"] = 0
                    changed = True
                normalized_members = []
                for member in members:
                    normalized_member = dict(member)
                    if "muted" not in normalized_member:
                        normalized_member["muted"] = False
                        changed = True
                    normalized_members.append(normalized_member)
                if normalized_members:
                    payload["members"] = normalized_members
                if not str(payload.get("name", "")).strip():
                    payload["name"] = next_name
                    changed = True
                elif previous_mode == "direct" and len(members) == 1:
                    payload["name"] = next_name
                    changed = True
                if not changed:
                    continue
                self._conn.execute(
                    "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                    (self._dump(payload), row["updated_at"], row["chat_id"]),
                )

    def _dump(self, payload: dict | list) -> str:
        return json.dumps(payload, ensure_ascii=False)

    def _load(self, payload: str) -> dict:
        return json.loads(payload)

    def _now(self) -> str:
        return datetime.now(tz=UTC).isoformat()

    def _get_payload(self, table: str, key_name: str, key_value: str) -> dict | None:
        row = self._conn.execute(
            f"SELECT payload FROM {table} WHERE {key_name} = ?",
            (key_value,),
        ).fetchone()
        if row is None:
            return None
        return self._load(row["payload"])

    def _chat_snapshot_locked(self, chat_id: str) -> dict | None:
        chat = self._get_payload("chats", "chat_id", chat_id)
        if chat is None:
            return None

        messages = [
            self._load(row["payload"])
            for row in self._conn.execute(
                """
                SELECT payload
                FROM chat_messages
                WHERE chat_id = ?
                ORDER BY created_at ASC, message_id ASC
                """,
                (chat_id,),
            ).fetchall()
        ]
        turns = [
            self._load(row["payload"])
            for row in self._conn.execute(
                """
                SELECT payload
                FROM turns
                WHERE chat_id = ?
                ORDER BY created_at ASC, turn_id ASC
                """,
                (chat_id,),
            ).fetchall()
        ]

        return {
            **chat,
            "members": list(chat.get("members", [])),
            "messages": messages,
            "turns": turns,
        }

    def upsert_node(self, node_id: str, payload: dict) -> dict:
        with self._lock, self._conn:
            now = self._now()
            current = self._get_payload("nodes", "node_id", node_id) or {}
            node = {
                **current,
                **payload,
                "last_seen_at": now,
            }
            self._conn.execute(
                """
                INSERT INTO nodes(node_id, payload, last_seen_at)
                VALUES(?, ?, ?)
                ON CONFLICT(node_id) DO UPDATE SET
                    payload = excluded.payload,
                    last_seen_at = excluded.last_seen_at
                """,
                (node_id, self._dump(node), now),
            )
            return deepcopy(node)

    def list_nodes(self) -> list[dict]:
        with self._lock:
            return [
                deepcopy(self._load(row["payload"]))
                for row in self._conn.execute(
                    "SELECT payload FROM nodes ORDER BY last_seen_at DESC, node_id ASC"
                ).fetchall()
            ]

    def get_node(self, node_id: str) -> dict | None:
        with self._lock:
            node = self._get_payload("nodes", "node_id", node_id)
            if node is None:
                return None
            return deepcopy(node)

    def delete_node(self, node_id: str) -> dict | None:
        with self._lock, self._conn:
            node = self._get_payload("nodes", "node_id", node_id)
            if node is None:
                return None
            self._conn.execute("DELETE FROM nodes WHERE node_id = ?", (node_id,))
            return deepcopy(node)

    def list_personas(self) -> list[dict]:
        with self._lock:
            return [
                deepcopy(self._load(row["payload"]))
                for row in self._conn.execute(
                    "SELECT payload FROM personas ORDER BY updated_at DESC, persona_id ASC"
                ).fetchall()
            ]

    def add_persona(self, payload: dict) -> dict:
        with self._lock, self._conn:
            now = self._now()
            persona = {
                "persona_id": f"persona_{uuid4().hex}",
                "status": "active",
                "model_provider": "codex",
                "created_at": now,
                "updated_at": now,
                **payload,
            }
            persona.pop("role_summary", None)
            self._conn.execute(
                """
                INSERT INTO personas(persona_id, payload, updated_at)
                VALUES(?, ?, ?)
                """,
                (persona["persona_id"], self._dump(persona), now),
            )
            return deepcopy(persona)

    def delete_persona(self, persona_id: str) -> dict | None:
        with self._lock, self._conn:
            persona = self._get_payload("personas", "persona_id", persona_id)
            if persona is None:
                return None
            self._conn.execute("DELETE FROM personas WHERE persona_id = ?", (persona_id,))
            return deepcopy(persona)

    def delete_chat(self, chat_id: str) -> dict | None:
        with self._lock, self._conn:
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            self._conn.execute("DELETE FROM chat_messages WHERE chat_id = ?", (chat_id,))
            self._conn.execute("DELETE FROM turns WHERE chat_id = ?", (chat_id,))
            self._conn.execute("DELETE FROM chats WHERE chat_id = ?", (chat_id,))
            return deepcopy(snapshot)

    def create_or_get_chat(self, personas: list[dict], *, name: str | None = None) -> dict:
        with self._lock, self._conn:
            requested_name = str(name or "").strip()
            default_name = "、".join(
                str(persona.get("name", "")).strip() for persona in personas[:3] if str(persona.get("name", "")).strip()
            ) or "新聊天"

            now = self._now()
            chat = {
                "chat_id": f"chat_{uuid4().hex}",
                "name": "、".join(persona.get("name", "") for persona in personas[:3]) or "新聊天",
                "mode": "group",
                "muted": False,
                "members": [
                    {
                        "persona_id": persona["persona_id"],
                        "name": persona.get("name", ""),
                        "status": persona.get("status", "active"),
                        "muted": False,
                    }
                    for persona in personas
                ],
                "created_at": now,
                "updated_at": now,
            }
            chat["name"] = requested_name or default_name
            chat["pinned"] = False
            chat["dnd"] = False
            chat["marked_unread"] = False
            chat["unread_count"] = 0
            self._conn.execute(
                """
                INSERT INTO chats(chat_id, payload, created_at, updated_at)
                VALUES(?, ?, ?, ?)
                """,
                (chat["chat_id"], self._dump(chat), now, now),
            )
            return deepcopy({**chat, "messages": [], "turns": []})

    def list_chats(self) -> list[dict]:
        with self._lock:
            chat_ids = [
                row["chat_id"]
                for row in self._conn.execute(
                    "SELECT chat_id FROM chats ORDER BY updated_at DESC, chat_id ASC"
                ).fetchall()
            ]
            snapshots = [
                deepcopy(snapshot)
                for chat_id in chat_ids
                if (snapshot := self._chat_snapshot_locked(chat_id)) is not None
            ]
            snapshots.sort(
                key=lambda chat: (
                    bool(chat.get("pinned", False)),
                    str(chat.get("updated_at", "")),
                    str(chat.get("chat_id", "")),
                ),
                reverse=True,
            )
            return snapshots

    def add_chat_message(
        self,
        chat_id: str,
        *,
        sender_type: str,
        sender_name: str,
        content: str,
        status: str = "completed",
        metadata: dict | None = None,
    ) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            created_at = self._now()
            message = {
                "message_id": f"msg_{uuid4().hex}",
                "sender_type": sender_type,
                "sender_name": sender_name,
                "content": content,
                "status": status,
                "created_at": created_at,
                "metadata": metadata or {},
            }
            self._conn.execute(
                """
                INSERT INTO chat_messages(message_id, chat_id, payload, created_at)
                VALUES(?, ?, ?, ?)
                """,
                (message["message_id"], chat_id, self._dump(message), created_at),
            )
            if sender_type == "agent":
                chat["unread_count"] = int(chat.get("unread_count", 0)) + 1
            elif sender_type == "user":
                chat["unread_count"] = 0
                chat["marked_unread"] = False
            chat["updated_at"] = created_at
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), created_at, chat_id),
            )
            return deepcopy(self._chat_snapshot_locked(chat_id))

    def update_chat(self, chat_id: str, payload: dict) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            now = self._now()
            chat.update(payload)
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            return deepcopy(chat)

    def clear_chat_history(self, chat_id: str) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            now = self._now()
            self._conn.execute("DELETE FROM chat_messages WHERE chat_id = ?", (chat_id,))
            self._conn.execute("DELETE FROM turns WHERE chat_id = ?", (chat_id,))
            chat["unread_count"] = 0
            chat["marked_unread"] = False
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            return deepcopy({**chat, "messages": [], "turns": []})

    def mark_chat_read(self, chat_id: str) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            now = self._now()
            chat["unread_count"] = 0
            chat["marked_unread"] = False
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            return deepcopy(chat)

    def add_members_to_chat(self, chat_id: str, personas: list[dict]) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            members = list(chat.get("members", []))
            existing_ids = {
                str(member.get("persona_id", "")).strip()
                for member in members
                if str(member.get("persona_id", "")).strip()
            }
            added_persona_ids: list[str] = []
            for persona in personas:
                persona_id = str(persona.get("persona_id", "")).strip()
                if not persona_id or persona_id in existing_ids:
                    continue
                members.append(
                    {
                        "persona_id": persona_id,
                        "name": str(persona.get("name", "")).strip(),
                        "status": str(persona.get("status", "active")).strip() or "active",
                        "muted": False,
                    }
                )
                existing_ids.add(persona_id)
                added_persona_ids.append(persona_id)

            if not added_persona_ids:
                snapshot = self._chat_snapshot_locked(chat_id)
                if snapshot is None:
                    return None
                return {"chat": deepcopy(snapshot), "added_persona_ids": []}

            now = self._now()
            chat["members"] = members
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            return {"chat": deepcopy(snapshot), "added_persona_ids": added_persona_ids}

    def remove_member_from_chat(self, chat_id: str, persona_id: str) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            members = [member for member in list(chat.get("members", [])) if isinstance(member, dict)]
            removed_member = next(
                (member for member in members if str(member.get("persona_id", "")).strip() == persona_id),
                None,
            )
            if removed_member is None:
                snapshot = self._chat_snapshot_locked(chat_id)
                if snapshot is None:
                    return None
                return {"chat": deepcopy(snapshot), "removed": False, "dissolved": False}

            remaining_members = [
                member for member in members if str(member.get("persona_id", "")).strip() != persona_id
            ]
            if not remaining_members:
                snapshot = self._chat_snapshot_locked(chat_id)
                self._conn.execute("DELETE FROM chat_messages WHERE chat_id = ?", (chat_id,))
                self._conn.execute("DELETE FROM turns WHERE chat_id = ?", (chat_id,))
                self._conn.execute("DELETE FROM chats WHERE chat_id = ?", (chat_id,))
                return {
                    "chat": deepcopy(snapshot) if snapshot is not None else None,
                    "removed": True,
                    "removed_member": deepcopy(removed_member),
                    "dissolved": True,
                }

            now = self._now()
            chat["members"] = remaining_members
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            return {
                "chat": deepcopy(snapshot),
                "removed": True,
                "removed_member": deepcopy(removed_member),
                "dissolved": False,
            }

    def set_member_muted(self, chat_id: str, persona_id: str, muted: bool) -> dict | None:
        with self._lock, self._conn:
            chat = self._get_payload("chats", "chat_id", chat_id)
            if chat is None:
                return None

            members = [member for member in list(chat.get("members", [])) if isinstance(member, dict)]
            target_member = None
            updated_members: list[dict] = []
            for member in members:
                current_persona_id = str(member.get("persona_id", "")).strip()
                if current_persona_id != persona_id:
                    updated_members.append(member)
                    continue

                target_member = {**member, "muted": bool(muted)}
                updated_members.append(target_member)

            if target_member is None:
                snapshot = self._chat_snapshot_locked(chat_id)
                if snapshot is None:
                    return None
                return {"chat": deepcopy(snapshot), "updated": False, "member": None}

            now = self._now()
            chat["members"] = updated_members
            chat["updated_at"] = now
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), now, chat_id),
            )
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            return {"chat": deepcopy(snapshot), "updated": True, "member": deepcopy(target_member)}

    def create_turn(
        self,
        chat_id: str,
        *,
        content: str,
        node_id: str | None,
        sender_name: str,
        status: str = "pending",
        extra: dict | None = None,
    ) -> dict | None:
        with self._lock, self._conn:
            if self._get_payload("chats", "chat_id", chat_id) is None:
                return None

            now = self._now()
            turn_id = f"turn_{uuid4().hex}"
            turn = {
                "turn_id": turn_id,
                "chat_id": chat_id,
                "content": content,
                "status": status,
                "assigned_node_id": node_id,
                "sender_name": sender_name,
                "created_at": now,
                "updated_at": now,
                **(extra or {}),
            }
            self._conn.execute(
                """
                INSERT INTO turns(turn_id, chat_id, payload, created_at, updated_at)
                VALUES(?, ?, ?, ?, ?)
                """,
                (turn_id, chat_id, self._dump(turn), now, now),
            )
            self._conn.execute(
                "UPDATE chats SET updated_at = ? WHERE chat_id = ?",
                (now, chat_id),
            )
            return deepcopy(turn)

    def update_turn(self, turn_id: str, payload: dict) -> dict | None:
        with self._lock, self._conn:
            turn = self._get_payload("turns", "turn_id", turn_id)
            if turn is None:
                return None

            now = self._now()
            turn.update(payload)
            turn["updated_at"] = now
            self._conn.execute(
                "UPDATE turns SET payload = ?, updated_at = ? WHERE turn_id = ?",
                (self._dump(turn), now, turn_id),
            )
            self._conn.execute(
                "UPDATE chats SET updated_at = ? WHERE chat_id = ?",
                (now, turn["chat_id"]),
            )
            return deepcopy(turn)

    def get_turn(self, turn_id: str) -> dict | None:
        with self._lock:
            turn = self._get_payload("turns", "turn_id", turn_id)
            if turn is None:
                return None
            return deepcopy(turn)

    def list_pending_turns_for_node(self, node_id: str) -> list[dict]:
        with self._lock:
            return [
                deepcopy(self._load(row["payload"]))
                for row in self._conn.execute(
                    """
                    SELECT payload
                    FROM turns
                    WHERE json_extract(payload, '$.assigned_node_id') = ?
                      AND json_extract(payload, '$.status') = 'pending'
                    ORDER BY created_at ASC, turn_id ASC
                    """,
                    (node_id,),
                ).fetchall()
            ]

    def chat_snapshot(self, chat_id: str) -> dict | None:
        with self._lock:
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            return deepcopy(snapshot)


def _resolve_state_store_path(data_dir: Path) -> Path:
    preferred = data_dir / "server-state"
    legacy = data_dir / "server.db"
    if preferred.exists() or not legacy.exists():
        return preferred

    for suffix in ("", "-wal", "-shm"):
        legacy_path = Path(f"{legacy}{suffix}")
        if not legacy_path.exists():
            continue
        try:
            legacy_path.rename(Path(f"{preferred}{suffix}"))
        except OSError:
            return legacy
    return preferred


state = StateStore(_resolve_state_store_path(Path(settings.server_data_dir).expanduser()))

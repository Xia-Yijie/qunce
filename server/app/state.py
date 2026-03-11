from __future__ import annotations

import json
import sqlite3
from copy import deepcopy
from datetime import UTC, datetime
from pathlib import Path
from threading import Lock
from uuid import uuid4

from server.app.config import settings


class SQLiteState:
    def __init__(self, db_path: Path) -> None:
        self._lock = Lock()
        self._db_path = db_path
        self._db_path.parent.mkdir(parents=True, exist_ok=True)
        self._conn = sqlite3.connect(self._db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._init_schema()

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
        turn_ids = [
            row["turn_id"]
            for row in self._conn.execute(
                """
                SELECT turn_id
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
            "turns": turn_ids,
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
            self._conn.execute(
                """
                INSERT INTO personas(persona_id, payload, updated_at)
                VALUES(?, ?, ?)
                """,
                (persona["persona_id"], self._dump(persona), now),
            )
            return deepcopy(persona)

    def list_chats(self) -> list[dict]:
        with self._lock:
            chat_ids = [
                row["chat_id"]
                for row in self._conn.execute(
                    "SELECT chat_id FROM chats ORDER BY updated_at DESC, chat_id ASC"
                ).fetchall()
            ]
            return [
                deepcopy(snapshot)
                for chat_id in chat_ids
                if (snapshot := self._chat_snapshot_locked(chat_id)) is not None
            ]

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
            chat["updated_at"] = created_at
            self._conn.execute(
                "UPDATE chats SET payload = ?, updated_at = ? WHERE chat_id = ?",
                (self._dump(chat), created_at, chat_id),
            )
            return deepcopy(self._chat_snapshot_locked(chat_id))

    def create_turn(self, chat_id: str, *, content: str, node_id: str | None, sender_name: str) -> dict | None:
        with self._lock, self._conn:
            if self._get_payload("chats", "chat_id", chat_id) is None:
                return None

            now = self._now()
            turn_id = f"turn_{uuid4().hex}"
            turn = {
                "turn_id": turn_id,
                "chat_id": chat_id,
                "content": content,
                "status": "queued",
                "assigned_node_id": node_id,
                "sender_name": sender_name,
                "created_at": now,
                "updated_at": now,
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

    def chat_snapshot(self, chat_id: str) -> dict | None:
        with self._lock:
            snapshot = self._chat_snapshot_locked(chat_id)
            if snapshot is None:
                return None
            return deepcopy(snapshot)


state = SQLiteState(Path(settings.server_data_dir).expanduser() / "server.db")

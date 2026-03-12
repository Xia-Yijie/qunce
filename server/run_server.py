from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parents[1]
if str(ROOT_DIR) not in sys.path:
    sys.path.insert(0, str(ROOT_DIR))

from server.app.routes import http as http_routes  # noqa: F401
from server.app.routes import http_chat as http_chat_routes  # noqa: F401
from server.app.routes import ws_agent as ws_agent_routes  # noqa: F401
from server.app.routes import ws_console as ws_console_routes  # noqa: F401
from server.app.runtime import app


def main() -> None:
    host = os.getenv("QUNCE_SERVER_HOST", "0.0.0.0")
    port = int(os.getenv("QUNCE_SERVER_PORT", "8000"))
    route_map = sorted(
        (rule.rule, sorted(rule.methods - {"HEAD", "OPTIONS"}))
        for rule in app.url_map.iter_rules()
        if rule.rule.startswith("/api/")
    )
    print(f"registered routes: {route_map}")
    app.run(host=host, port=port, debug=False)


if __name__ == "__main__":
    main()

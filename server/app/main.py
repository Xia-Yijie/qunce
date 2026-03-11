from __future__ import annotations

import os

from server.app.routes import http as http_routes  # noqa: F401
from server.app.routes import ws_agent as ws_agent_routes  # noqa: F401
from server.app.routes import ws_console as ws_console_routes  # noqa: F401
from server.app.runtime import app


def main() -> None:
    host = os.getenv("QUNCE_SERVER_HOST", "0.0.0.0")
    port = int(os.getenv("QUNCE_SERVER_PORT", "8000"))
    app.run(host=host, port=port, debug=False)


if __name__ == "__main__":
    main()

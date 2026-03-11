from __future__ import annotations

from pathlib import Path

from flask import Flask
from flask_sock import Sock

BASE_DIR = Path(__file__).resolve().parents[2]
DIST_DIR = BASE_DIR / "console" / "dist"

app = Flask(__name__, static_folder=None)
sock = Sock(app)

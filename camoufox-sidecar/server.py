"""
Camoufox sidecar — Pinchtab-compatible HTTP API wrapping stealth Firefox.
Exposes the same endpoint contract as Pinchtab so Go can swap clients seamlessly.
"""

import os
import threading
from typing import Any

import uvicorn
from camoufox.sync_api import Camoufox
from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel

PORT = int(os.environ.get("CAMOUFOX_PORT", "9868"))
TOKEN = os.environ.get("CAMOUFOX_TOKEN", "changeme")

app = FastAPI()
security = HTTPBearer()

# Single persistent browser + page, protected by a lock for sequential access
_lock = threading.Lock()
_browser = None
_page = None


def _get_browser():
    global _browser, _page
    if _browser is None:
        cf = Camoufox(headless=False, humanize=True)
        _browser = cf.__enter__()
        _page = _browser.new_page()
    return _browser, _page


def require_auth(credentials: HTTPAuthorizationCredentials = Depends(security)):
    if credentials.credentials != TOKEN:
        raise HTTPException(status_code=401, detail="invalid token")


# ── Health ──────────────────────────────────────────────────────────────────

@app.get("/health")
def health():
    return {"status": "ok"}


# ── Navigate ─────────────────────────────────────────────────────────────────

class NavigateRequest(BaseModel):
    url: str


@app.post("/navigate")
def navigate(body: NavigateRequest, _: None = Depends(require_auth)):
    with _lock:
        _, page = _get_browser()
        page.goto(body.url, wait_until="domcontentloaded", timeout=30_000)
    return {"ok": True}


# ── Get text ─────────────────────────────────────────────────────────────────

@app.get("/text")
def get_text(_: None = Depends(require_auth)):
    with _lock:
        _, page = _get_browser()
        text = page.inner_text("body")
    return {"text": text}


# ── Cookies ──────────────────────────────────────────────────────────────────

class CookieItem(BaseModel):
    name: str
    value: str
    domain: str
    path: str = "/"


@app.post("/cookies")
def inject_cookies(cookies: list[CookieItem], _: None = Depends(require_auth)):
    with _lock:
        _, page = _get_browser()
        context = page.context
        context.add_cookies([c.model_dump() for c in cookies])
    return {"ok": True}


# ── Evaluate JS ──────────────────────────────────────────────────────────────

class EvaluateRequest(BaseModel):
    expression: str


@app.post("/evaluate")
def evaluate(body: EvaluateRequest, _: None = Depends(require_auth)):
    with _lock:
        _, page = _get_browser()
        result = page.evaluate(body.expression)
    # Pinchtab returns {"result": "<string>"}
    return {"result": result if isinstance(result, str) else str(result)}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="info")

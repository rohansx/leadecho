"""
Camoufox sidecar — Pinchtab-compatible HTTP API wrapping stealth Firefox.
Exposes the same endpoint contract as Pinchtab so Go can swap clients seamlessly.

Browser runs in a dedicated subprocess (not in the asyncio thread pool) to avoid
Playwright Sync API conflicts with FastAPI's event loop.
"""

import os
import queue
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

# ── Browser worker thread ───────────────────────────────────────────────────
# Playwright Sync API cannot run inside asyncio's thread pool. We run the
# browser in a plain thread with its own message queue, and the FastAPI
# handlers send commands to it and wait for results.

_cmd_queue: "queue.Queue[dict]" = queue.Queue()
_result_queue: "queue.Queue[dict]" = queue.Queue()
_browser_ready = threading.Event()
_browser_error: str | None = None


def _browser_worker():
    """Runs in a dedicated thread — owns the Camoufox/Playwright instance.
    Auto-recovers if the browser or page crashes."""
    global _browser_error
    cf = None
    browser = None
    page = None

    def _init():
        nonlocal cf, browser, page
        cf = Camoufox(headless=True, humanize=False)
        browser = cf.__enter__()
        page = browser.new_page()

    def _reinit():
        nonlocal cf, browser, page
        try:
            if cf:
                cf.__exit__(None, None, None)
        except Exception:
            pass
        cf = None
        browser = None
        page = None
        _init()

    try:
        _init()
        _browser_ready.set()

        while True:
            cmd = _cmd_queue.get()
            if cmd["action"] == "shutdown":
                break

            try:
                if cmd["action"] == "navigate":
                    # Check if page is still alive, reinit if not
                    try:
                        page.evaluate("1")
                    except Exception:
                        _reinit()
                    page.goto(cmd["url"], wait_until="domcontentloaded", timeout=30_000)
                    _result_queue.put({"ok": True})
                elif cmd["action"] == "text":
                    text = page.inner_text("body")
                    _result_queue.put({"ok": True, "text": text})
                elif cmd["action"] == "cookies":
                    context = page.context
                    context.add_cookies(cmd["cookies"])
                    _result_queue.put({"ok": True})
                elif cmd["action"] == "evaluate":
                    result = page.evaluate(cmd["expression"])
                    _result_queue.put({"ok": True, "result": result if isinstance(result, str) else str(result)})
                else:
                    _result_queue.put({"ok": False, "error": f"unknown action: {cmd['action']}"})
            except Exception as e:
                _result_queue.put({"ok": False, "error": str(e)})

        if cf:
            cf.__exit__(None, None, None)
    except Exception as e:
        _browser_error = str(e)
        _browser_ready.set()


# Start the browser worker at import time
_thread = threading.Thread(target=_browser_worker, daemon=True)
_thread.start()


def _send_cmd(cmd: dict, timeout: float = 45) -> dict:
    """Send a command to the browser worker and wait for the result."""
    if not _browser_ready.wait(timeout=30):
        raise HTTPException(status_code=503, detail="camoufox browser not ready")
    if _browser_error:
        raise HTTPException(status_code=503, detail=f"camoufox init failed: {_browser_error}")
    _cmd_queue.put(cmd)
    try:
        result = _result_queue.get(timeout=timeout)
    except queue.Empty:
        raise HTTPException(status_code=504, detail="camoufox operation timed out")
    if not result.get("ok", False):
        raise HTTPException(status_code=500, detail=result.get("error", "unknown error"))
    return result


def require_auth(credentials: HTTPAuthorizationCredentials = Depends(security)):
    if credentials.credentials != TOKEN:
        raise HTTPException(status_code=401, detail="invalid token")


# ── Health ──────────────────────────────────────────────────────────────────

@app.get("/health")
def health():
    if _browser_ready.is_set() and _browser_error is None:
        return {"status": "ok"}
    if _browser_error:
        return {"status": "error", "detail": _browser_error}
    return {"status": "starting"}


# ── Navigate ─────────────────────────────────────────────────────────────────

class NavigateRequest(BaseModel):
    url: str


@app.post("/navigate")
def navigate(body: NavigateRequest, _: None = Depends(require_auth)):
    _send_cmd({"action": "navigate", "url": body.url})
    return {"ok": True}


# ── Get text ─────────────────────────────────────────────────────────────────

@app.get("/text")
def get_text(_: None = Depends(require_auth)):
    result = _send_cmd({"action": "text"})
    return {"text": result["text"]}


# ── Cookies ──────────────────────────────────────────────────────────────────

class CookieItem(BaseModel):
    name: str
    value: str
    domain: str
    path: str = "/"


@app.post("/cookies")
def inject_cookies(cookies: list[CookieItem], _: None = Depends(require_auth)):
    _send_cmd({"action": "cookies", "cookies": [c.model_dump() for c in cookies]})
    return {"ok": True}


# ── Evaluate JS ──────────────────────────────────────────────────────────────

class EvaluateRequest(BaseModel):
    expression: str


@app.post("/evaluate")
def evaluate(body: EvaluateRequest, _: None = Depends(require_auth)):
    result = _send_cmd({"action": "evaluate", "expression": body.expression})
    return {"result": result["result"]}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="info")

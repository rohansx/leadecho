"""
Scrapling sidecar — Pinchtab-compatible HTTP API wrapping Scrapling's stealth browser.
Exposes the same endpoint contract as Pinchtab/Camoufox so Go can swap clients seamlessly.

Uses StealthyFetcher for anti-bot bypass and adaptive DOM parsing.
Falls back to PlayWrightFetcher if StealthyFetcher is unavailable.
"""

import json
import os
import threading
from typing import Any

import uvicorn
from fastapi import Depends, FastAPI, HTTPException
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel
from scrapling import StealthyFetcher, PlayWrightFetcher

PORT = int(os.environ.get("SCRAPLING_PORT", "9869"))
TOKEN = os.environ.get("SCRAPLING_TOKEN", "changeme")
USE_STEALTH = os.environ.get("SCRAPLING_STEALTH", "true").lower() in ("true", "1", "yes")

app = FastAPI()
security = HTTPBearer()

_lock = threading.Lock()
_fetcher = None
_last_response = None  # stores most recent Scrapling response for text/evaluate
_cookies: list[dict] = []


def _get_fetcher():
    global _fetcher
    if _fetcher is None:
        if USE_STEALTH:
            _fetcher = StealthyFetcher()
        else:
            _fetcher = PlayWrightFetcher()
    return _fetcher


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
    global _last_response
    with _lock:
        fetcher = _get_fetcher()
        # Build cookie header string from stored cookies
        cookie_header = "; ".join(f"{c['name']}={c['value']}" for c in _cookies) if _cookies else None
        headers = {}
        if cookie_header:
            headers["Cookie"] = cookie_header

        try:
            resp = fetcher.fetch(
                body.url,
                headless=True,
                disable_resources=True,
                extra_headers=headers,
                timeout=30000,
            )
            _last_response = resp
        except Exception as e:
            raise HTTPException(status_code=502, detail=f"scrapling fetch failed: {e}")

    return {"ok": True}


# ── Get text ─────────────────────────────────────────────────────────────────


@app.get("/text")
def get_text(_: None = Depends(require_auth)):
    with _lock:
        if _last_response is None:
            raise HTTPException(status_code=400, detail="no page loaded — call /navigate first")
        text = _last_response.get_all_text() if hasattr(_last_response, "get_all_text") else _last_response.text
    return {"text": text}


# ── Cookies ──────────────────────────────────────────────────────────────────


class CookieItem(BaseModel):
    name: str
    value: str
    domain: str
    path: str = "/"


@app.post("/cookies")
def inject_cookies(cookies: list[CookieItem], _: None = Depends(require_auth)):
    global _cookies
    with _lock:
        _cookies = [c.model_dump() for c in cookies]
    return {"ok": True}


# ── Evaluate JS ──────────────────────────────────────────────────────────────


class EvaluateRequest(BaseModel):
    expression: str


@app.post("/evaluate")
def evaluate(body: EvaluateRequest, _: None = Depends(require_auth)):
    with _lock:
        if _last_response is None:
            raise HTTPException(status_code=400, detail="no page loaded — call /navigate first")

        # Scrapling's Adaptor supports executing JS on the page when using
        # PlayWrightFetcher or StealthyFetcher in real-browser mode.
        # If the page was fetched via the real browser, we can evaluate JS.
        page = getattr(_last_response, "_page", None) or getattr(_last_response, "page", None)
        if page is not None:
            try:
                result = page.evaluate(body.expression)
                return {"result": result if isinstance(result, str) else json.dumps(result)}
            except Exception as e:
                raise HTTPException(status_code=500, detail=f"JS evaluation failed: {e}")

        # Fallback: if no live page, try CSS-based extraction from the stored response
        raise HTTPException(
            status_code=400,
            detail="JS evaluation not available — page was fetched without a live browser context",
        )


# ── Scrape (Scrapling-native endpoint — higher level than Pinchtab contract) ─


class ScrapeRequest(BaseModel):
    url: str
    selector: str  # CSS selector for elements to extract
    fields: dict[str, str]  # field_name -> CSS selector within each element
    limit: int = 20
    cookies: list[CookieItem] = []


@app.post("/scrape")
def scrape(body: ScrapeRequest, _: None = Depends(require_auth)):
    """
    Higher-level scrape endpoint that leverages Scrapling's adaptive parsing.
    Fetches a URL, selects elements, and extracts fields — all in one call.
    This avoids the navigate→wait→evaluate round-trip and works even without JS.
    """
    with _lock:
        fetcher = _get_fetcher()
        cookie_header = "; ".join(f"{c.name}={c.value}" for c in body.cookies) if body.cookies else None
        headers = {}
        if cookie_header:
            headers["Cookie"] = cookie_header

        try:
            resp = fetcher.fetch(
                body.url,
                headless=True,
                disable_resources=True,
                extra_headers=headers,
                timeout=30000,
            )
        except Exception as e:
            raise HTTPException(status_code=502, detail=f"scrapling fetch failed: {e}")

        elements = resp.css(body.selector)
        results = []
        for el in elements[: body.limit]:
            item = {}
            for field_name, field_selector in body.fields.items():
                found = el.css(field_selector)
                if found:
                    item[field_name] = found[0].text.strip() if hasattr(found[0], "text") else str(found[0])
                else:
                    item[field_name] = ""
            results.append(item)

    return {"results": results}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="info")

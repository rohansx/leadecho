"""LeadEcho identity-engine tool server (runs inside the E2B sandbox).

Read-only research tools behind a bearer token. Stdlib HTTP; each tool runs in a
subprocess so its timeout is a hard kill.

    python3 server.py                -> serve on :8000
    python3 server.py browse <url>   -> internal, used by POST /browse
"""
import glob
import hmac
import ipaddress
import json
import os
import re
import urllib.parse
import socket
import subprocess
import sys
import tempfile
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urljoin, urlsplit

PORT = 8000
MAX_BODY = 8 * 1024
MAX_CHARS = 20_000
MAX_LINKS = 300
# The start command is snapshotted at template build, so create-time envVars never
# reach this process. The token comes from env (manual runs) or from this file,
# which the caller writes through envd right after creating the sandbox.
TOKEN_FILE = "/home/user/.tool_token"
USERNAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$")
URL_RE = re.compile(r"https?://[^\s<>\"')\]]+")

# Curated professional/dev sites (names as in maigret's data.json). Never the full db.
SITES = [
    "GitHub", "GitLab", "BitBucket", "codeberg.org", "Twitter", "Bluesky",
    "mastodon.social", "Threads", "Instagram", "DEV Community", "Medium",
    "Substack", "Hashnode", "Keybase", "ProductHunt", "HackerNews", "Lobsters",
    "Reddit", "Kaggle", "HuggingFace", "StackOverflow", "LeetCode", "Hackerrank",
    "Codewars", "Behance", "Dribbble", "Codepen", "Replit", "Observable", "NPM",
    "Docker Hub", "YouTube", "Twitch", "Telegram", "Patreon",
    "BuyMeACoffee", "kofi", "Gumroad", "Gravatar", "About.me", "linktr.ee",
    "Calendly", "Speakerdeck", "SlideShare", "sessionize.com",
]  # PyPi, Polywork, F6S are disabled in maigret's db (0.6.7)

_token = None


class ToolError(Exception):
    def __init__(self, status, msg):
        super().__init__(msg)
        self.status = status


def tool_token():
    global _token
    if _token:
        return _token
    tok = os.environ.get("TOOL_TOKEN", "").strip()
    if not tok:
        try:
            with open(TOKEN_FILE) as f:
                tok = f.read().strip()
        except OSError:
            return None
    if len(tok) >= 16:  # ponytail: first valid token wins for the sandbox's life
        _token = tok
    return _token


def run(cmd, timeout):
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, stdin=subprocess.DEVNULL)
    except subprocess.TimeoutExpired:
        raise ToolError(504, f"timed out after {timeout}s")
    return p


# ---------- SSRF guard ----------

def _public_ip(ip):
    a = ipaddress.ip_address(ip)
    return a.is_global and not a.is_multicast


def check_url(url):
    if not isinstance(url, str) or len(url) > 2048:
        raise ToolError(400, "url must be a string <= 2048 chars")
    u = urlsplit(url)
    if u.scheme not in ("http", "https") or not u.hostname:
        raise ToolError(400, "only http(s) URLs are allowed")
    host_is_public(u.hostname, strict=True)
    return url


def host_is_public(host, strict=False):
    try:
        infos = socket.getaddrinfo(host, None)
    except socket.gaierror:
        if strict:
            raise ToolError(400, f"cannot resolve {host}")
        return False
    ok = all(_public_ip(i[4][0]) for i in infos)
    if strict and not ok:
        raise ToolError(400, "private, loopback or reserved addresses are not allowed")
    return ok
    # ponytail: no DNS-rebinding pinning; Chromium re-resolves. Pin via --host-resolver-rules if it matters.


# ---------- tools ----------

def username_sweep(body):
    username = body.get("username")
    if not isinstance(username, str) or not USERNAME_RE.match(username):
        raise ToolError(400, "username must match " + USERNAME_RE.pattern)
    with tempfile.TemporaryDirectory() as out:
        cmd = ["maigret", username, "--no-autoupdate", "--no-recursion", "--no-progressbar",
               "--no-color", "--timeout", "12", "--retries", "0", "-J", "simple", "-fo", out]
        for s in SITES:
            cmd += ["--site", s]
        p = run(cmd, 90)
        reports = glob.glob(os.path.join(out, "*.json"))
        if not reports:
            raise ToolError(502, "maigret produced no report: " + (p.stderr or p.stdout)[-500:])
        with open(reports[0]) as f:
            data = json.load(f)
    found = []
    for site, r in data.items():
        st = r.get("status") or {}
        if st.get("status") != "Claimed":
            continue
        ids = {k: v for k, v in (st.get("ids") or {}).items() if not k.startswith("_")}
        found.append({"site": site, "url": st.get("url") or r.get("url_user"),
                      "status": "found", "ids": ids})
    return {"username": username, "sites_checked": len(SITES), "found": cap_list(found)}


def cap_list(items):
    out, size = [], 0
    for it in items:
        size += len(json.dumps(it))
        if size > MAX_CHARS:
            break
        out.append(it)
    return out


def browse(body):
    url = check_url(body.get("url"))
    p = run([sys.executable, os.path.abspath(__file__), "browse", url], 30)
    if p.returncode != 0:
        raise ToolError(502, (p.stderr or "browse failed")[-500:])
    return json.loads(p.stdout)


def browse_child(url):
    """Runs in its own process; killed by the parent on timeout."""
    from playwright.sync_api import sync_playwright

    allowed = {}

    def guard(route):
        u = urlsplit(route.request.url)
        if u.scheme in ("data", "blob"):
            return route.continue_()
        if u.scheme not in ("http", "https") or not u.hostname:
            return route.abort()
        if u.hostname not in allowed:
            allowed[u.hostname] = host_is_public(u.hostname)
        return route.continue_() if allowed[u.hostname] else route.abort()

    with sync_playwright() as pw:
        b = pw.chromium.launch(headless=True)
        ctx = b.new_context(user_agent="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130 Safari/537.36")
        ctx.route("**/*", guard)
        page = ctx.new_page()
        page.goto(url, wait_until="domcontentloaded", timeout=20_000)
        try:
            page.wait_for_load_state("networkidle", timeout=4_000)
        except Exception:
            pass  # ponytail: best effort; busy pages still return DOM-ready text
        text = page.inner_text("body") if page.query_selector("body") else ""
        raw = page.eval_on_selector_all(
            "a[href]", "els => els.map(e => [e.getAttribute('href'), (e.innerText||'').trim(), e.getAttribute('rel')||''])")
        final = page.url
        title = page.title()
        b.close()
    links, seen = [], set()
    for href, txt, rel in raw:
        absu = urljoin(final, href).split("#")[0]
        if not absu.startswith(("http://", "https://")) or absu in seen:
            continue
        seen.add(absu)
        links.append({"href": absu, "text": txt[:200], "rel": rel})
        if len(links) >= MAX_LINKS:
            break
    text = re.sub(r"\n\s*\n+", "\n\n", text).strip()
    return {"final_url": final, "title": title, "text": text[:MAX_CHARS], "links": links}


CHANNEL_RE = re.compile(r"youtube\.com/(@[^/?#]+|channel/[^/?#]+|c/[^/?#]+|user/[^/?#]+)/?$")


YOUTUBE_HOSTS = {"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be"}


def youtube(body):
    url = check_url(body.get("url"))
    # yt-dlp follows redirects and resolves hosts itself, past check_url: only
    # YouTube URLs, so it never fetches an internal address.
    if urllib.parse.urlsplit(url).hostname not in YOUTUBE_HOSTS:
        raise ToolError(400, "only youtube.com URLs")
    cmd = ["yt-dlp", "-J", "--flat-playlist", "--playlist-end", "15", "--skip-download", "--no-warnings",
           "--no-playlist", "--"]
    deadline = time.monotonic() + 60
    p = None
    if CHANNEL_RE.search(url.split("?")[0]):
        p = run(cmd + [url.rstrip("/") + "/videos"], 45)  # channels with no videos tab fail here
    if p is None or p.returncode != 0:
        p = run(cmd + [url], max(5, int(deadline - time.monotonic())))
    if p.returncode != 0:
        raise ToolError(502, (p.stderr or "yt-dlp failed")[-500:])
    d = json.loads(p.stdout)
    desc = d.get("description") or ""
    out = {
        "type": d.get("_type", "video"),
        "title": d.get("title"),
        "channel": d.get("channel") or d.get("uploader"),
        "channel_id": d.get("channel_id"),
        "channel_url": d.get("channel_url") or d.get("uploader_url"),
        "uploader_id": d.get("uploader_id"),
        "followers": d.get("channel_follower_count"),
        "description": desc[:8000],
        "links": sorted(set(URL_RE.findall(desc)))[:100],
        "upload_date": d.get("upload_date"),
        "webpage_url": d.get("webpage_url"),
    }
    entries = d.get("entries") or []
    if entries:
        out["recent_videos"] = [{"title": e.get("title"), "url": e.get("url"),
                                 "views": e.get("view_count")} for e in entries[:15]]
    return out


ROUTES = {"/username_sweep": username_sweep, "/browse": browse, "/youtube": youtube}


class Handler(BaseHTTPRequestHandler):
    server_version = "identity-engine"
    sys_version = ""

    def reply(self, status, obj):
        b = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def authed(self):
        tok = tool_token()
        if not tok:
            self.reply(503, {"error": "tool token not provisioned"})
            return False
        got = self.headers.get("Authorization", "")
        if not hmac.compare_digest(got.encode(), ("Bearer " + tok).encode()):
            self.reply(401, {"error": "unauthorized"})
            return False
        return True

    def do_GET(self):
        if self.path == "/health":
            return self.reply(200, {"ok": True, "token_ready": tool_token() is not None})
        if self.authed():
            self.reply(404, {"error": "not found"})

    def do_POST(self):
        if not self.authed():
            return
        fn = ROUTES.get(self.path)
        if not fn:
            return self.reply(404, {"error": "not found"})
        try:
            n = int(self.headers.get("Content-Length") or 0)
            if n <= 0 or n > MAX_BODY:
                raise ToolError(400, "JSON body required (<= 8KB)")
            body = json.loads(self.rfile.read(n))
            if not isinstance(body, dict):
                raise ToolError(400, "body must be a JSON object")
            t = time.monotonic()
            res = fn(body)
            res["elapsed_ms"] = int((time.monotonic() - t) * 1000)
            self.reply(200, res)
        except ToolError as e:
            self.reply(e.status, {"error": str(e)})
        except json.JSONDecodeError:
            self.reply(400, {"error": "invalid JSON"})
        except Exception as e:  # noqa: BLE001 - surface tool crashes as 500, keep serving
            self.log_error("tool crash: %r", e)
            self.reply(500, {"error": f"{type(e).__name__}: {str(e)[:300]}"})


def main():
    if len(sys.argv) == 3 and sys.argv[1] == "browse":
        print(json.dumps(browse_child(sys.argv[2])))
        return
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()


if __name__ == "__main__":
    main()

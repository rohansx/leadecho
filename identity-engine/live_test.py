"""Live end-to-end check using only raw HTTP (what the Go side will do).
    .venv/bin/python live_test.py [--private]
--private creates the sandbox with network.allowPublicTraffic=false and sends
e2b-traffic-access-token on every port-8000 call.
"""
import json
import os
import secrets
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from dotenv import load_dotenv

load_dotenv(Path(__file__).parent.parent / ".env")
API = "https://api.e2b.app"
KEY = os.environ["E2B_API_KEY"]
PRIVATE = "--private" in sys.argv
SECRET_FIELDS = ("envdAccessToken", "trafficAccessToken")


def http(method, url, body=None, headers=None, raw=None, timeout=120):
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    h = {"Content-Type": "application/json", **(headers or {})}
    req = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            txt = r.read().decode()
            return r.status, (json.loads(txt) if txt.strip() else None)
    except urllib.error.HTTPError as e:
        txt = e.read().decode()
        try:
            return e.code, json.loads(txt)
        except ValueError:
            return e.code, txt[:300]


def show(label, status, obj, n=900):
    print(f"--- {label}: HTTP {status}\n{json.dumps(obj, indent=1)[:n]}\n")


def main():
    token = secrets.token_urlsafe(32)
    body = {"templateID": "leadecho-identity", "timeout": 300, "envVars": {"TOOL_TOKEN": token}}
    if PRIVATE:
        body["network"] = {"allowPublicTraffic": False}
    t0 = time.monotonic()
    st, sb = http("POST", f"{API}/sandboxes", body, {"X-API-Key": KEY})
    t_create = time.monotonic() - t0
    redacted = {k: ("<redacted>" if k in SECRET_FIELDS and v else v) for k, v in (sb or {}).items()} if isinstance(sb, dict) else sb
    show(f"create ({t_create:.2f}s)", st, redacted)
    if st != 201:
        return
    sid, domain = sb["sandboxID"], sb.get("domain") or "e2b.app"
    base = f"https://8000-{sid}.{domain}"
    envd = f"https://49983-{sid}.{domain}"
    traffic = {"e2b-traffic-access-token": sb["trafficAccessToken"]} if PRIVATE else {}
    try:
        st, h = http("GET", base + "/health", headers=traffic)
        show(f"health before token (+{time.monotonic() - t0:.2f}s since create start)", st, h)
        if PRIVATE:
            st, h = http("GET", base + "/health")
            show("health WITHOUT traffic token (private sandbox)", st, h)

        t1 = time.monotonic()
        st, r = http("POST", f"{envd}/files?path=/home/user/.tool_token&username=user", raw=token.encode(),
                     headers={"X-Access-Token": sb["envdAccessToken"], "Content-Type": "application/octet-stream"})
        show(f"envd write token ({time.monotonic() - t1:.2f}s)", st, r)
        st, h = http("GET", base + "/health", headers=traffic)
        show(f"health after token (cold start total {time.monotonic() - t0:.2f}s)", st, h)

        auth = {**traffic, "Authorization": f"Bearer {token}"}
        st, r = http("POST", base + "/username_sweep", {"username": "simonw"}, headers=traffic)
        show("sweep, no bearer", st, r)
        st, r = http("POST", base + "/username_sweep", {"username": "simonw"},
                     headers={**traffic, "Authorization": "Bearer wrong"})
        show("sweep, wrong bearer", st, r)
        for path, payload in [("/username_sweep", {"username": "simonw"}),
                              ("/browse", {"url": "https://simonwillison.net"}),
                              ("/browse", {"url": "https://simonwillison.net/about/"}),
                              ("/youtube", {"url": "https://www.youtube.com/@3blue1brown"}),
                              ("/browse", {"url": "http://169.254.169.254/latest/meta-data/"}),
                              ("/browse", {"url": "file:///etc/passwd"})]:
            t = time.monotonic()
            st, r = http("POST", base + path, payload, headers=auth)
            if isinstance(r, dict) and "text" in r:
                r = {**r, "text": r["text"][:400], "links": r["links"][:5], "n_links": len(r["links"])}
            if isinstance(r, dict) and "found" in r:
                r = {**r, "found": r["found"][:4], "found_sites": [f["site"] for f in r["found"]]}
            if isinstance(r, dict) and "recent_videos" in r:
                r = {**r, "recent_videos": r["recent_videos"][:3], "description": r["description"][:200]}
            show(f"{path} {payload} ({time.monotonic() - t:.2f}s)", st, r, 2500)
    finally:
        t = time.monotonic()
        st, r = http("DELETE", f"{API}/sandboxes/{sid}", headers={"X-API-Key": KEY})
        show(f"kill ({time.monotonic() - t:.2f}s)", st, r)


if __name__ == "__main__":
    main()

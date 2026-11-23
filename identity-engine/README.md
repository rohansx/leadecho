# identity-engine (E2B template `leadecho-identity`)

A sandbox that runs read-only person-research tools behind a small HTTP server
(`server.py`, stdlib only, port 8000). The LLM loop runs in Go; Go starts a sandbox,
calls these tools, then kills it. The image also has `gh` and
[Agent-Reach](https://github.com/Panniantong/Agent-Reach) (the `agent-reach` package on PyPI is a different project), which the server does not expose.

- Template ID `rxkjjhpgg9f71v5w22tv`, name/alias `leadecho-identity`, 2 vCPU / 2 GB.
- Built with E2B's current Python `Template()` SDK (`e2b==2.64.0`), not `e2b.Dockerfile`.

## Rebuild

```sh
uv venv .venv && uv pip install --python .venv/bin/python 'e2b==2.64.0' python-dotenv
.venv/bin/python template.py      # E2B_API_KEY from ../.env, ~35s cached / ~1.5 min cold
.venv/bin/python live_test.py     # raw-REST end-to-end check; add --private to test the traffic token
```

The start command runs `server.py` during the build and is **snapshotted**. A
new sandbox already has the server listening when the create call returns.

## The tool token (important)

Because the server process is snapshotted at build time, **`envVars` passed at
sandbox create never reach it** (E2B docs: "not available to the start command
process"). The server reads `TOOL_TOKEN` from its env (for manual runs) or, in
practice, from `/home/user/.tool_token`. Go writes that file through envd right
after create. The server keeps the first valid token (16 chars or more) it finds. Until a token is
present, every authed endpoint returns `503 {"error":"tool token not provisioned"}`
and `/health` returns `token_ready:false`.

## REST calls from Go

API base `https://api.e2b.app`, auth header `X-API-Key: <E2B_API_KEY>`.

**1. Create**: `POST /sandboxes`
```json
{"templateID": "leadecho-identity", "timeout": 300,
 "network": {"allowPublicTraffic": false}}
```
`timeout` is in seconds. Response `201`:
```json
{"sandboxID": "i7vzzeb5sn396cdoyjfuc", "templateID": "rxkjjhpgg9f71v5w22tv",
 "alias": "leadecho-identity", "clientID": "6532622b", "envdVersion": "0.10.2",
 "envdAccessToken": "...", "trafficAccessToken": "...", "domain": null}
```
`trafficAccessToken` only appears when `allowPublicTraffic` is false. If `domain` is null, use `e2b.app`.

**2. Provision the tool token**: write it through envd (port 49983):
```
POST https://49983-<sandboxID>.e2b.app/files?path=/home/user/.tool_token&username=user
X-Access-Token: <envdAccessToken>
Content-Type: application/octet-stream
<body: the token bytes>
```
Response `200 [{"name":".tool_token","path":"/home/user/.tool_token","type":"file"}]`.
Use 32 or more random bytes, for example base64url.

**3. Call tools**: `https://8000-<sandboxID>.<domain|e2b.app>/<endpoint>`
- `Authorization: Bearer <token>` on everything except `GET /health`.
- `e2b-traffic-access-token: <trafficAccessToken>` on **every** request, `/health`
  included, when the sandbox was created with `allowPublicTraffic:false`.
  Without it the E2B edge returns `403 {"code":403,"message":"Sandbox is secured with traffic access token. ..."}`.
  With the default (public) setting the URL is open to anyone who knows the sandbox ID,
  and only the bearer token protects it. Prefer private.

**4. Kill**: `DELETE /sandboxes/<sandboxID>` with `X-API-Key`. Response `204`, empty body.

Measured (Nov 2026): create takes about 2s, the first `/health` 200 comes about 4.5s after the create request,
and the server is ready with its token about 5.5–7s after the create request. The first sandbox after a fresh build is slower
(about 13s). Kill takes about 0.5s.

## Endpoints

Errors are always `{"error": "..."}` with 400 (bad input or blocked URL), 401, 404, 502
(tool failed), 503 (no token), 504 (hard timeout), or 500. Request bodies are JSON objects of 8 KB or less.
Successful responses carry `elapsed_ms`.

| Endpoint | Request | Response |
|---|---|---|
| `GET /health` | (no auth) | `{"ok":true,"token_ready":bool}` |
| `POST /username_sweep` | `{"username":"simonw"}` (`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`) | `{"username","sites_checked":45,"found":[{"site","url","status":"found","ids":{...maigret-extracted fields: fullname, bio, location, blog_url, twitter_username, follower_count, created_at...}}]}` |
| `POST /browse` | `{"url":"https://..."}` | `{"final_url","title","text"(≤20k chars),"links":[{"href","text","rel"}](absolute, deduped, ≤300)}` |
| `POST /youtube` | `{"url": video or channel URL}` | `{"type","title","channel","channel_id","channel_url","uploader_id","followers","description"(≤8k),"links":[urls in description],"upload_date","webpage_url","recent_videos"?:[{"title","url","views"}](≤15, channels)}` |

Limits:
- **username_sweep**: Maigret 0.6.7 over the curated `SITES` list in `server.py`, never the full database. It runs with `--no-autoupdate --no-recursion`, a 12s timeout per site and a 90s hard limit, and typically takes about 20s. `found` is capped at roughly 20k chars of JSON. Maigret also returns some derived entries such as `GitHubGist` and `Libraries`. Matches are only "username exists", and many are other people with the same handle, so the LLM must corroborate them.
- **browse**: Playwright Chromium, headless, 30s hard limit, about 5s typical. Only http(s) is allowed. The target and every subrequest must resolve to a public IP, so private, loopback, link-local and metadata addresses are refused.
- **youtube**: `yt-dlp -J --flat-playlist`, metadata only, 60s hard limit, about 2–4s typical. A channel URL goes to `/videos` first and falls back to the bare URL for channels with no videos tab.

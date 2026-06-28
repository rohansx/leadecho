# Pinchtab Sidecar Configuration Governance Fix

> author: Chris Chen 
> Affected components: `docker-compose.yml`, `pinchtab/config.json` (new), `.env`
> Severity: P0 — authenticated crawling pipeline fully non-functional

## 1. Symptoms

After starting the leadecho backend, logs continuously emitted three classes of errors. The authenticated crawling pipeline (Reddit/Twitter/LinkedIn/Quora) was completely down:

```
✗ pinchtab heartbeat: status 401
✗ Failed to navigate to reddit: pinchtab /navigate: status 403: {"error":"navigation blocked by IDPI: Domain not in allowlist: www.reddit.com"}
WRN reddit-pinchtab: failed to decrypt session error="decrypt: cipher: message authentication failed"
```

## 2. Root Cause Analysis

Investigation revealed **three independent root causes** that compounded to sever the entire sidecar pipeline.

### Root Cause 1: Volume Mount Path Mismatch (the master cause of config non-persistence)

**Symptom**: After every `docker compose up --force-recreate`, pinchtab's security configuration (allowedDomains, allowCookies, etc.) reverted to defaults.

**Root cause**: The volume was mounted to the wrong path in `docker-compose.yml`:

```yaml
# Incorrect configuration
volumes:
  - pinchtab_profiles:/root/.pinchtab   # pinchtab runs as user 'pinchtab', HOME=/data
```

The pinchtab image runs as the `pinchtab` user (uid=1000) with `HOME=/data`, reading/writing `/data/.pinchtab/config.json`. But the volume was mounted at `/root/.pinchtab` (root's home directory), which the `pinchtab` user cannot access — **Permission denied**.

**Consequences**:
- config.json was actually written to `/data/.pinchtab` inside the container layer (not on the persistent volume)
- Container recreation → config lost → entrypoint script re-ran `pinchtab config init` → generated a random token + maximally strict security defaults
- All manual `pinchtab config set` modifications were wiped on every rebuild

### Root Cause 2: Pinchtab Default Security Profile Mismatched with Crawling Use Case

`pinchtab config init` generates a **maximum-security default profile** (designed for AI-agent browser automation, not web crawling):

| Config key | Default | Impact on leadecho |
|---|---|---|
| `security.allowedDomains` | `["127.0.0.1","localhost","::1"]` | All external site navigation returns 403 |
| `security.idpi.strictMode` | `true` | Non-allowlisted domains rejected outright |
| `security.allowCookies` | `false` | `/cookies` endpoint disabled → **cannot inject login cookies** |
| `security.allowEvaluate` | `false` | `/evaluate` endpoint disabled → **cannot verify login state** |

The backend (`pinchtab.go`) uses three pinchtab endpoints, two of which were disabled:
- `POST /navigate` — navigation (works after allowlisting)
- `POST /cookies` — inject login cookies (`allowCookies: false` → 403)
- `POST /evaluate` — execute JS to verify login state (`allowEvaluate: false` → 403)

The core authenticated-crawling chain is: user pastes cookies → backend injects into pinchtab → verify login state → crawl with session. With `allowCookies` and `allowEvaluate` disabled, the entire chain from injection to verification was broken.

### Root Cause 3: Three-Way Token Mismatch

The pinchtab entrypoint script logic:

```sh
if [ ! -f config.json ]; then
  pinchtab config init                    # generates a random token
  if [ -n "${PINCHTAB_TOKEN:-}" ]; then   # only overrides if env is non-empty
    pinchtab config set server.token "$PINCHTAB_TOKEN"
  fi
fi
```

In `docker-compose.yml`, the `PINCHTAB_TOKEN` env line was commented out → env was empty → entrypoint skipped the override → pinchtab used the randomly generated token (e.g., `bdd8f5b5...`).

The backend read `PINCHTAB_TOKEN=changeme` from `.env` and sent it as a Bearer token → token mismatch → `401 Unauthorized`.

### Incidental Issue: Encryption Key Mismatch Causing Session Decryption Failures

The backend uses `ENCRYPTION_KEY` (falling back to `JWT_SECRET` when empty) to AES-256-GCM encrypt user-pasted cookies before storing them in `platform_accounts.access_token_enc`.

`.env` had `JWT_SECRET=change-this-to-a-random-secret-in-production`, but existing encrypted sessions in the database were encrypted with the code default `leadecho-dev-secret-change-in-prod` → GCM authentication failed → `cipher: message authentication failed`.

## 3. Fix Strategy

Adopted a **"Configuration as Code"** architecture — shifting pinchtab configuration from "generated at container runtime" to "version-controlled in the repository + rendered at startup."

### Design Principles

1. **Single source of truth**: `pinchtab/config.json` is the sole configuration definition, reviewed and deployed alongside code
2. **Secret-config separation**: Sensitive values (token) are never hardcoded in the config file; they flow through `.env` → docker-compose → sed rendering at startup
3. **State-config separation**: Browser profiles (runtime state) use a named volume; config.json (declarative config) is bind-mounted read-only
4. **Idempotent startup**: Whether `docker compose down -v` or `--force-recreate`, the configuration is always consistent after container start

### Implementation

#### New file: `pinchtab/config.json` — declarative config template

Exported the running config and adapted it for version control:
- `server.token`: `"PINCHTAB_TOKEN_PLACEHOLDER"` (plain-string placeholder, replaced by sed at startup)
- `browser.version`: cleared (don't pin Chrome version; let pinchtab auto-match)
- `instanceDefaults.maxTabs`: 5 (aligned with docker-compose env)
- `instanceDefaults.stealthLevel`: `full` (crawling requires maximum stealth)
- `security.allowedDomains`: added 8 crawling target domains
- `security.allowCookies`: `true` (required for authenticated crawling)
- `security.allowEvaluate`: `true` (required for login-state verification)
- `security.idpi.strictMode/scanContent/wrapContent`: `false` (prevent DOM tampering that corrupts crawl extraction)

#### Modified: `docker-compose.yml` — render-at-startup injection

```yaml
pinchtab:
  image: pinchtab/pinchtab:latest
  user: root                                    # start as root to fix volume permissions
  entrypoint: ["sh", "-c"]
  command:
    - |
      mkdir -p /data/.pinchtab && \
      chown -R pinchtab:pinchtab /data/.pinchtab && \
      sed 's|PINCHTAB_TOKEN_PLACEHOLDER|${PINCHTAB_TOKEN}|g' /tmpl/config.json > /data/.pinchtab/config.json && \
      chown pinchtab:pinchtab /data/.pinchtab/config.json && \
      exec su pinchtab -c "exec pinchtab server"   # drop privileges back to pinchtab
  volumes:
    - ./pinchtab/config.json:/tmpl/config.json:ro    # config template (read-only)
    - pinchtab_state:/data/.pinchtab/profiles         # browser state (persistent)
```

**Key design decisions**:

- `user: root` + `chown -R` + `su pinchtab`: Docker named volumes create mount-point directories as root, which the `pinchtab` user cannot write to. Starting as root fixes permissions, then `su` drops back to the unprivileged user for the actual server process — balancing permission repair with least-privilege principle.
- `sed` placeholder replacement: docker-compose interpolates `${PINCHTAB_TOKEN}` from `.env` at the YAML layer, producing a sed command that renders the template into runtime config. A plain-string placeholder (`PINCHTAB_TOKEN_PLACEHOLDER`) is used instead of `${VAR}` syntax to avoid the triple-escaping hell of YAML + shell + docker-compose variable interpolation.
- Volume mounts only `/data/.pinchtab/profiles` (browser state), not `/data/.pinchtab` itself (to prevent the volume from shadowing the rendered config.json).

#### Modified: `.env` — token and keys

- `PINCHTAB_TOKEN`: generated a random 24-byte hex (`openssl rand -hex 24`), consistent across all three parties (.env, docker-compose env, pinchtab config)
- Cleared all existing encrypted sessions in `platform_accounts` (the encryption key had changed; old data is cryptographically unrecoverable)

## 4. Issues Encountered and Resolved

### Issue 1: The `$${VAR}` Escaping Nightmare

**Attempt**: Used `sed "s|$${PINCHTAB_TOKEN}|${PINCHTAB_TOKEN}|g"` in the command, expecting `$$` to escape to a literal `$` in docker-compose.

**Result**: docker-compose rendered `$$` as `$$` (not `$`), so the shell received `$${PINCHTAB_TOKEN}` = `$$` + `${PINCHTAB_TOKEN}` → the pattern became `$$38998...` → didn't match `${PINCHTAB_TOKEN}` in the template.

**Resolution**: Switched to a plain-string placeholder `PINCHTAB_TOKEN_PLACEHOLDER`, completely sidestepping the ambiguity of shell/docker-compose variable syntax.

### Issue 2: Volume Mount Root-Owning Parent Directories

**Symptom**: Volume mounted at `/data/.pinchtab/profiles` → Docker created the parent directory `/data/.pinchtab` as root → the `pinchtab` user couldn't create `config.json` inside it (Permission denied).

**Attempt**: Mounted the entire `/data/.pinchtab` → same root-ownership issue → `pinchtab` user couldn't write to `profiles/` → Chrome couldn't create its user-data-dir → instance startup failed → `/navigate` returned 503.

**Resolution**: `user: root` at startup + `chown -R pinchtab:pinchtab /data/.pinchtab` (recursive permission fix) + `su pinchtab` to drop privileges before running the server.

### Issue 3: Browser Instance Not Starting After `su` Privilege Drop

**Symptom**: `user: root` + `su pinchtab -c "exec pinchtab server"` → health returned 200 but `/navigate` returned 503 (`instance not ready after 10s`).

**Root cause**: `chown` fixed only `/data/.pinchtab` itself, not the `profiles/` subdirectory (the volume mount point, still root-owned) → `pinchtab` user couldn't write to profiles → Chrome couldn't launch.

**Resolution**: `chown -R` (recursive) instead of `chown`.

### Issue 4: `docker compose --profile browser up` Not Starting Non-Profile Services

**Symptom**: `docker compose --profile browser up -d` only started the browser-profile service (pinchtab); postgres/redis (no profile) didn't start → API couldn't connect to the database.

**Resolution**: Run `docker compose up -d` first (base services), then `docker compose --profile browser up -d` (overlay browser profile).

## 5. Verification Results

```
=== Post force-recreate config state ===
token: 38998393dbbba151...   ← rendered from .env
allowCookies: True            ← persisted
allowEvaluate: True           ← persisted
strictMode: False             ← persisted
domains: 11                   ← 8 crawling domains + 3 localhost

=== Functional verification ===
health:          200
navigate reddit: 200
cookies:         200
```

After `docker compose up --force-recreate`, the configuration no longer resets — Root Cause 1 (volume path mismatch) is permanently resolved.

## 6. Architectural Reflection

### Why not use `docker exec pinchtab config set` for each setting?

This was the approach used early in the fix. It failed because:
1. **Not idempotent**: Depends on the container existing; everything is lost after `docker compose down -v`
2. **Not declarative**: Configuration is scattered across shell history / runbooks; not reviewable, not traceable
3. **Violates Infrastructure-as-Code**: Production configuration should come from version control, not operator memory

### Why not use `PINCHTAB_CONFIG` env to specify the config path?

pinchtab supports `PINCHTAB_CONFIG=/path/to/config.json`. Theoretically, the template could be bind-mounted there. But:
1. The pinchtab main program still interacts with the entrypoint script's `if [ ! -f default_config_path ]` logic
2. No token rendering (pinchtab doesn't parse `${VAR}` syntax inside config.json)
3. A bind-mounted file is read-only; pinchtab's runtime `save config` attempts would fail

### Production Evolution Direction

The current approach suits a monorepo/small team. As scale grows, it should evolve toward:

| Current | Evolution |
|---|---|
| Single pinchtab sidecar | Playwright cluster on k8s (elastic scaling) |
| Versioned config.json | ConfigMap + Secret (k8s-native config management) |
| sed token rendering | envsubst sidecar / Vault Agent injection |
| Hardcoded allowedDomains | Egress proxy (Squid/Envoy) for unified domain allowlisting |
| AES-encrypted in DB | HashiCorp Vault / cloud KMS for session key management |
| `JWT_SECRET` in .env | Secret manager (Vault / AWS Secrets Manager / Doppler) |

## 7. Files Changed

| File | Change |
|---|---|
| `pinchtab/config.json` | **Added** — declarative configuration template |
| `docker-compose.yml` | pinchtab service block rewritten (entrypoint/command/volumes/user) |
| `.env` | `PINCHTAB_TOKEN` updated to random 24-byte hex |

FROM python:3.12-slim

# System deps for Playwright/Chromium (used by Scrapling's StealthyFetcher)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 \
    libxkbcommon0 libxcomposite1 libxdamage1 libxrandr2 libgbm1 \
    libpango-1.0-0 libcairo2 libasound2 libxshmfence1 \
    && rm -rf /var/lib/apt/lists/*

# Install from requirements.txt so the pins there actually apply. Installing the
# packages inline meant requirements.txt was never read, and every build silently
# picked up the newest scrapling.
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Scrapling asks browserforge for header fingerprints at import time. On Linux,
# browserforge 1.2.4 (latest) only ships Chrome data up to version 141, but
# scrapling 0.4.0 requests 143 — so the import raises "No headers based on this
# input can be generated" and the container exits(1) on every start, before
# serving a single request.
#
# Clamp the two version constants to what the bundled dataset actually covers.
# Written as sed rather than a heredoc: this image builds with the classic
# builder, where Dockerfile heredocs are not interpreted and the patch would
# silently no-op. Drop this once browserforge ships newer Chrome fingerprints.
RUN FP=$(python -c "import scrapling.engines.toolbelt.fingerprints as f; print(f.__file__)") \
    && sed -i -E 's/^chromium_version[[:space:]]*=[[:space:]]*[0-9]+/chromium_version = 141/' "$FP" \
    && sed -i -E 's/^chrome_version[[:space:]]*=[[:space:]]*[0-9]+/chrome_version = 141/' "$FP" \
    && grep -E '^(chromium_version|chrome_version)' "$FP"

RUN python -c "import scrapling.engines._browsers._config_tools; print('scrapling imports cleanly')"

# Install Scrapling's browser dependencies (Camoufox stealth browser + Playwright)
RUN scrapling install

WORKDIR /app
COPY server.py .

CMD ["python", "server.py"]

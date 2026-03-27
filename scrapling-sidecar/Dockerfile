FROM python:3.12-slim

# System deps for Playwright/Chromium (used by Scrapling's StealthyFetcher)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 \
    libxkbcommon0 libxcomposite1 libxdamage1 libxrandr2 libgbm1 \
    libpango-1.0-0 libcairo2 libasound2 libxshmfence1 \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir scrapling[all] fastapi "uvicorn[standard]"

# Install Scrapling's browser dependencies (Camoufox stealth browser + Playwright)
RUN scrapling install

WORKDIR /app
COPY server.py .

CMD ["python", "server.py"]

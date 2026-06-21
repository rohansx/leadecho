FROM python:3.12-slim

# Keep pip install and camoufox fetch FIRST so Docker cache is preserved
# across Dockerfile edits (these layers are ~700MB and slow to rebuild)
RUN pip install camoufox fastapi "uvicorn[standard]"
RUN python -m camoufox fetch

# System libs needed by Camoufox Firefox — placed after cached layers
# so adding/changing these doesn't invalidate the 700MB download above
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgtk-3-0 \
    libdbus-glib-1-2 \
    libxt6 \
    libasound2 \
    libpci3 \
    libx11-xcb1 \
    libxcb-shm0 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libxfixes3 \
    libxext6 \
    libxrender1 \
    libfontconfig1 \
    libfreetype6 \
    libpango-1.0-0 \
    libcairo2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY server.py .
CMD ["python", "server.py"]

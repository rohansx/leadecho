"""E2B template `leadecho-identity`: build with
    .venv/bin/python template.py      (reads E2B_API_KEY from ../.env)
"""
from pathlib import Path

from dotenv import load_dotenv
from e2b import Template, default_build_logger, wait_for_url

HERE = Path(__file__).parent
NAME = "leadecho-identity"

GH_APT = (
    "curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg "
    "-o /usr/share/keyrings/githubcli-archive-keyring.gpg && "
    'echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] '
    'https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list && '
    "apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*"
)

template = (
    Template(file_context_path=HERE)
    .from_image("python:3.12-bookworm")
    .set_envs({"PLAYWRIGHT_BROWSERS_PATH": "/ms-playwright", "PYTHONUNBUFFERED": "1"})
    .run_cmd(GH_APT, user="root")
    .run_cmd(
        "pip install --no-cache-dir maigret==0.6.7 yt-dlp playwright "
        "git+https://github.com/Panniantong/Agent-Reach",
        user="root",
    )
    .run_cmd("playwright install --with-deps chromium && chmod -R a+rX /ms-playwright", user="root")
    .copy("server.py", "/app/server.py", user="root", mode=0o755)
    # Warm Chromium once before serving so its pages are hot in the snapshot.
    .set_start_cmd(
        "python3 /app/server.py browse 'data:text/html,warm' >/dev/null; python3 /app/server.py",
        wait_for_url("http://localhost:8000/health"),
    )
)

if __name__ == "__main__":
    load_dotenv(HERE.parent / ".env")
    info = Template.build(template, NAME, cpu_count=2, memory_mb=2048, on_build_logs=default_build_logger())
    print(info)

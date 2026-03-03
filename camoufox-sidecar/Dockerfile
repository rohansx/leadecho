FROM python:3.12-slim
RUN pip install camoufox fastapi "uvicorn[standard]"
RUN python -m camoufox fetch
WORKDIR /app
COPY server.py .
CMD ["python", "server.py"]

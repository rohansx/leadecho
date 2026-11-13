# Debian-based builder: it already has git and the CA bundle, so the build
# does not depend on Alpine's package mirror (which hung production deploys
# for over an hour on 2026-11-13).
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/leadecho-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/leadecho-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/leadecho-api /leadecho-api
COPY --from=builder /app/leadecho-worker /leadecho-worker
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8090 8091 9090
USER nonroot:nonroot
ENTRYPOINT ["/leadecho-api"]

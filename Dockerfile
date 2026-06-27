# syntax=docker/dockerfile:1

# ---- builder: compile the Go binary ----
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/claude-api-comm ./cmd/server

# ---- runtime: Node (for the Claude CLI) + git + the binary ----
FROM node:22-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code

COPY --from=builder /out/claude-api-comm /usr/local/bin/claude-api-comm

# claude reads credentials from $HOME/.claude — mount the host's ~/.claude to /root/.claude at run time.
ENV HOME=/root \
    WORKSPACE_BASE_DIR=/sessions \
    LISTEN_ADDR=:18100
RUN mkdir -p /sessions

EXPOSE 18100
ENTRYPOINT ["claude-api-comm"]

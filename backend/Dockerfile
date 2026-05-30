FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /server ./cmd/server

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache git ca-certificates openssh-keygen tzdata

# Non-root user for security
RUN addgroup -S gitpier && adduser -S gitpier -G gitpier

COPY --from=builder /server /server

# Data directories are mounted via PersistentVolumeClaims at runtime.
# The SSH host key is expected at $SSH_HOST_KEY_PATH (default /data/ssh/ssh_host_key).
RUN mkdir -p /data/repos /data/avatars /data/packages /data/markdown-assets \
             /data/ssh /data/workflow-workspaces && \
    chown -R gitpier:gitpier /data

USER gitpier

EXPOSE 8080 2222

ENTRYPOINT ["/server"]

FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

RUN corepack enable
RUN corepack prepare pnpm@9.15.4 --activate

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm run build

FROM golang:1.25-alpine AS backend-builder

RUN apk add --no-cache git

WORKDIR /app/backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache git ca-certificates openssh-keygen tzdata

RUN addgroup -S gitpier && adduser -S gitpier -G gitpier

COPY --from=backend-builder /server /server
COPY --from=frontend-builder /app/frontend/build /app/frontend/build
COPY action-runner /app/action-runner

RUN mkdir -p /app/frontend/build \
	/app/action-runner \
	/data/repos \
	/data/avatars \
	/data/packages \
	/data/markdown-assets \
	/data/ssh \
	/data/workflow-workspaces && \
	chown -R gitpier:gitpier /app/frontend /app/action-runner /data

USER gitpier

ENV FRONTEND_DIST_PATH=/app/frontend/build

EXPOSE 8828 2424

ENTRYPOINT ["/server"]

# syntax=docker/dockerfile:1

# ---- Stage 1: build Svelte frontend ----
FROM node:20-alpine AS web-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci || npm install
COPY web/ .
RUN npm run build

# ---- Stage 2: build Go binary ----
FROM golang:1.25-alpine AS go-builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# The built SPA is dropped into web/dist so `//go:embed` compiles it into the
# binary — the final image carries no separate frontend directory.
COPY --from=web-builder /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/gogitit ./cmd/gogitit

# ---- Stage 3: final image ----
# Alpine, not distroless: GoGitIt shells out to the system `git` for worktree
# and index operations (internal/gitext), so the binary must be on PATH.
FROM alpine:3.20
RUN apk add --no-cache git ca-certificates \
    && adduser -D -u 10001 app
WORKDIR /app
COPY --from=go-builder /out/gogitit /app/gogitit
EXPOSE 8080
USER app
ENTRYPOINT ["/app/gogitit"]

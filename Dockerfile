FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./internal/webui/dist
# Defaults to "dev" (matching the Go source's own default) when not passed;
# a real build sets it explicitly: docker build --build-arg VERSION=v0.x.y .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o /rakitsu ./cmd/rakitsu

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1000 rakitsu
COPY --from=builder /rakitsu /usr/local/bin/rakitsu
WORKDIR /workspace
RUN chown rakitsu:rakitsu /workspace
USER rakitsu
ENTRYPOINT ["rakitsu"]

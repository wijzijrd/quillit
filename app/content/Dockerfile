# Build stage — compile the Go binary (no Go toolchain needed on the host)
#
# Build context is the repo root (see infra/docker-compose.yml: `context: ..`
# from infra/, dockerfile: app/content/Dockerfile), because go.mod's local
# replace directives need sibling-of-app/ modules in scope:
# `replace github.com/quillit/contentengine => ../../pkg/contentengine`
# and `replace github.com/quillit/gen => ../../gen` (the connectrpc-
# generated client/server code content's RPC boundary imports). All COPY
# sources below are therefore relative to the repo root, and the module is
# placed at /src/app/content so those go.mod replace paths
# (../../pkg/contentengine, ../../gen) resolve to /src/pkg/contentengine and
# /src/gen inside the image.
FROM golang:1.27-alpine AS build
WORKDIR /src/app/content
COPY app/content/go.mod app/content/go.sum ./
COPY pkg/contentengine /src/pkg/contentengine
COPY gen /src/gen
RUN go mod download
COPY app/content/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-content-svc .

# Runtime stage
FROM alpine:3.24
RUN mkdir -p /data
COPY --from=build /out/quillit-content-svc /usr/local/bin/quillit-content-svc
ENV PORT=3004
ENV DB_PATH=/data/quillit-content.db
EXPOSE 3004
CMD ["quillit-content-svc"]

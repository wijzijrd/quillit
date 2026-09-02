# Build stage — compile the Go binary (no Go toolchain needed on the host)
#
# Build context is the repo root (see infra/docker-compose.yml: `context: ..`
# from infra/, dockerfile: app/svc/Dockerfile), because go.mod's local
# replace directives need sibling-of-app/ modules in scope:
# `replace github.com/quillit/contentengine => ../../pkg/contentengine`
# (an unused replace today — nothing in svc currently imports contentengine,
# see below) and `replace github.com/quillit/gen => ../../gen` (the
# connectrpc-generated client/server code svc's RPC boundary imports). All
# COPY sources below are therefore relative to the repo root, and the module
# is placed at /src/app/svc so those go.mod replace paths
# (../../pkg/contentengine, ../../gen) resolve to /src/pkg/contentengine and
# /src/gen inside the image.
FROM golang:1.27-alpine AS build
WORKDIR /src/app/svc
COPY app/svc/go.mod app/svc/go.sum ./
COPY pkg/contentengine /src/pkg/contentengine
COPY gen /src/gen
RUN go mod download
COPY app/svc/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-svc .

# Runtime stage
FROM alpine:3.24
RUN mkdir -p /data
COPY --from=build /out/quillit-svc /usr/local/bin/quillit-svc
ENV PORT=3000
ENV DB_PATH=/data/quillit.db
ENV COOKIE_SECURE=true
EXPOSE 3000
CMD ["quillit-svc"]

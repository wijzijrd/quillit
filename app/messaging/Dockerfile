# Build stage — compile the Go binary (no Go toolchain needed on the host)
#
# Build context is the repo root (see infra/docker-compose.yml: `context: ..`
# from infra/, dockerfile: app/messaging/Dockerfile), because go.mod's local
# replace directive needs a sibling-of-app/ module in scope:
# `replace github.com/quillit/gen => ../../gen` (the connectrpc-generated
# client/server code messaging's RPC boundary imports). All COPY sources
# below are therefore relative to the repo root, and the module is placed
# at /src/app/messaging so that go.mod replace path (../../gen) resolves to
# /src/gen inside the image.
FROM golang:1.27-alpine AS build
WORKDIR /src/app/messaging
COPY app/messaging/go.mod app/messaging/go.sum ./
COPY gen /src/gen
RUN go mod download
COPY app/messaging/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-messaging-svc .

# Runtime stage
FROM alpine:3.24
COPY --from=build /out/quillit-messaging-svc /usr/local/bin/quillit-messaging-svc
ENV PORT=3003
EXPOSE 3003
CMD ["quillit-messaging-svc"]

# Build stage — compile the Go binary (no Go toolchain needed on the host)
#
# Build context is the repo root (see infra/docker-compose.yml: `context: ..`
# from infra/, dockerfile: app/auth/Dockerfile), because go.mod's local
# replace directive needs a sibling-of-app/ module in scope:
# `replace github.com/quillit/gen => ../../gen` (the connectrpc-generated
# client/server code auth's RPC boundary imports). All COPY sources below
# are therefore relative to the repo root, and the module is placed at
# /src/app/auth so that go.mod replace path (../../gen) resolves to
# /src/gen inside the image.
FROM golang:1.27-alpine AS build
WORKDIR /src/app/auth
COPY app/auth/go.mod app/auth/go.sum ./
COPY gen /src/gen
RUN go mod download
COPY app/auth/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-auth-svc .

# Runtime stage
FROM alpine:3.24
RUN mkdir -p /data
COPY --from=build /out/quillit-auth-svc /usr/local/bin/quillit-auth-svc
ENV PORT=3002
ENV DB_PATH=/data/quillit-auth.db
EXPOSE 3002
CMD ["quillit-auth-svc"]

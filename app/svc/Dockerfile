# Build stage — compile the Go binary (no Go toolchain needed on the host)
#
# Build context is the repo root (see infra/docker-compose.yml: `context: ..`
# from infra/, dockerfile: app/svc/Dockerfile), because go.mod's
# `replace github.com/quillit/contentengine => ../../pkg/contentengine`
# needs pkg/contentengine, a sibling of app/, in scope. All COPY sources
# below are therefore relative to the repo root, and the module is placed
# at /src/app/svc so the go.mod replace path (../../pkg/contentengine)
# resolves to /src/pkg/contentengine inside the image.
FROM golang:1.27-alpine AS build
WORKDIR /src/app/svc
COPY app/svc/go.mod app/svc/go.sum ./
COPY pkg/contentengine /src/pkg/contentengine
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

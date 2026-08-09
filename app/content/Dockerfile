# Build stage — compile the Go binary (no Go toolchain needed on the host)
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY ../../pkg/contentengine ../../pkg/contentengine
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-content-svc .

# Runtime stage
FROM alpine:3.24
RUN mkdir -p /data
COPY --from=build /out/quillit-content-svc /usr/local/bin/quillit-content-svc
ENV PORT=3004
ENV DB_PATH=/data/quillit-content.db
EXPOSE 3004
CMD ["quillit-content-svc"]

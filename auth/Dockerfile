# Build stage — compile the Go binary (no Go toolchain needed on the host)
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-auth-svc .

# Runtime stage
FROM alpine:3.21
RUN mkdir -p /data
COPY --from=build /out/quillit-auth-svc /usr/local/bin/quillit-auth-svc
ENV PORT=3002
ENV DB_PATH=/data/quillit-auth.db
EXPOSE 3002
CMD ["quillit-auth-svc"]

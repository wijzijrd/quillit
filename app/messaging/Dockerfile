# Build stage — compile the Go binary (no Go toolchain needed on the host)
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/quillit-messaging-svc .

# Runtime stage
FROM alpine:3.24
COPY --from=build /out/quillit-messaging-svc /usr/local/bin/quillit-messaging-svc
ENV PORT=3003
EXPOSE 3003
CMD ["quillit-messaging-svc"]

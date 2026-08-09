# Build stage — compile the Go binary (no Go toolchain needed on the host)
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
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

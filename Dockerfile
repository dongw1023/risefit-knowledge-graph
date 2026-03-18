# --- Build Stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build both binaries
RUN go build -o ingest ./cmd/ingest
RUN go build -o server ./cmd/server

# --- Ingest Stage ---
FROM alpine:latest AS ingest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/ingest .
ENTRYPOINT ["./ingest"]

# --- Server Stage ---
FROM alpine:latest AS server
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
ENTRYPOINT ["./server"]

# Stage 1: Build the Go binaries
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git for potential private modules
RUN apk add --no-cache git

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Build both binaries
RUN go build -o ingest ./cmd/ingest
RUN go build -o server ./cmd/server

# Stage 2: Create a minimal runtime image
FROM alpine:latest

# Install CA certificates for HTTPS requests (needed for OpenAI/Google API calls)
RUN apk add --no-cache ca-certificates

# Set working directory
WORKDIR /root/

# Copy binaries from the builder stage
COPY --from=builder /app/ingest .
COPY --from=builder /app/server .

# Expose the API server port (default 8080)
EXPOSE 8080

# The default command will start the server
# You can override this to run "ingest" when needed:
# docker run --env-file .env my-app ./ingest path/to/file.pdf
CMD ["./server"]

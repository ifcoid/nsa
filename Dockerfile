# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Stamp commit nsa ke binary (Reproducible Error: /diagnostic melaporkan backend_version).
# Di-pass dari CI: flyctl deploy --build-arg VERSION_COMMIT=${{ github.sha }}. Default "dev".
ARG VERSION_COMMIT=dev

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X 'nsa/internal/version.Commit=${VERSION_COMMIT}'" \
    -o main ./cmd/app/...

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Expose port
EXPOSE 50607

# Run the application
CMD ["./main"]
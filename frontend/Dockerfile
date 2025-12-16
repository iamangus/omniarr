# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code explicitly
COPY cmd ./cmd
COPY internal ./internal
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -o omniarr-frontend ./cmd/omniarr-frontend

# Run Stage
FROM scratch

WORKDIR /app

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the Pre-built binary
COPY --from=builder /app/omniarr-frontend .

# Copy templates directory
COPY --from=builder /app/internal/templates ./internal/templates

# Expose port 8080
EXPOSE 8080

# Command to run the executable
CMD ["./omniarr-frontend"]
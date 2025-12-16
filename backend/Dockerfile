# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code explicitly to ensure they are present
COPY cmd ./cmd
COPY internal ./internal
RUN mkdir config
# Copy any other necessary files
COPY . .

# Build the Go app
# Added ls to debug if it fails again, but explicit COPY should catch missing files earlier
RUN CGO_ENABLED=0 GOOS=linux go build -o omniarr ./cmd/omniarr

# Run Stage
FROM scratch

WORKDIR /app

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the Pre-built binary
COPY --from=builder /app/omniarr .

# Expose port 8080
EXPOSE 8080

# Command to run the executable
CMD ["./omniarr"]
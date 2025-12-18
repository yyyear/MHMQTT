# Build stage
FROM golang:alpine AS builder

# Set environment variables
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=https://goproxy.cn
# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o mhmqtt main.go

# Final stage
FROM alpine:latest

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mhmqtt .

# Copy config file
COPY config.yaml .

# Create directories for data and logs
RUN mkdir -p data logs

# Expose ports based on config.yaml
# 1883: MQTT, 8083: WebSocket, 8080: HTTP API
EXPOSE 1883 8083 8080

# Run the application
CMD ["./mhmqtt"]

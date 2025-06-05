# Build stage
FROM golang:1.24-alpine AS builder

# Set necessary environmet variables needed for our image
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -a -installsuffix cgo -o ironic-metadata ./cmd/ironic-metadata

# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/ironic-metadata .

# Build a small image
FROM alpine:latest

# Add ca-certificates in case we need to make HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder stage
COPY --from=builder /dist/ironic-metadata .

# Expose port
EXPOSE 80

# Command to run when starting the container
CMD ["./ironic-metadata"]

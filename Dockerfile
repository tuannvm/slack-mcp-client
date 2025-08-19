# BuildKit ARGs for cross-platform builds
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies in a separate layer for better caching
RUN apk add --no-cache git

# Copy go mod files first for efficient dependency caching
COPY go.mod go.sum ./

# Download dependencies using build cache
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Copy source code
COPY . .

# Cross-compile with build caching
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -trimpath -o slack-mcp-client ./cmd/

# Minimal final image
FROM --platform=$TARGETPLATFORM alpine:3.22

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/slack-mcp-client .

# Command to run
ENTRYPOINT ["./slack-mcp-client"] 

# Copyright 2026 Zen Mesh
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build stage
# Builder toolchain pinned by digest (golang:1.27.1-alpine). The module
# declares `go 1.27.0`; the release contract (project.yaml goVersion, README)
# declares Go 1.27 images. NOTE: .github workflow go-version pins remain 1.26.6
# pending CI ownership (SCOUT-019: .github is out of scope by law).
FROM golang@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build optimized binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Build for target architecture (defaults to linux/amd64 for single-arch builds)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w \
        -X 'main.version=${VERSION}' \
        -X 'main.commit=${COMMIT}' \
        -X 'main.buildDate=${BUILD_DATE}'" \
    -o zen-cleaner ./cmd/zen-cleaner

# Runtime stage - use scratch (empty) base for minimal size
# The binary is statically linked (CGO_ENABLED=0), so no libc needed
FROM scratch

# Copy CA certificates from Alpine for HTTPS/TLS support (needed for Kubernetes API)
# This is much smaller than the full Alpine base (~200KB vs 8MB)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/zen-cleaner /zen-cleaner

# Run as a non-root user (scratch has no user database; numeric ID required).
# The in-cluster Helm/securityContext also enforces runAsNonRoot — this makes
# the image safe by default even when run bare.
USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/zen-cleaner"]


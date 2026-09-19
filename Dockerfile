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
# Builder toolchain is pinned to the exact go.mod / CI version: the module
# declares `go 1.26.6`, CI builds with 1.26.6, and the release contract
# (project.yaml goVersion, README) declares Go 1.26 images.
FROM golang:1.26.6-alpine AS builder

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

EXPOSE 8080

ENTRYPOINT ["/zen-cleaner"]


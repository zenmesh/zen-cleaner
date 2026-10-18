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
# Builder toolchain pinned by digest (golang:1.27.2-ALPINE). The module
# declares `go 1.27.2`; the release contract (project.yaml goVersion, README,
# .github workflow go-version pins) all declare Go 1.27.
#
# THE BASE VARIANT IS PART OF THE PIN, not an incidental detail. The build
# stage runs `apk add`, so the builder MUST be an Alpine image. Dependabot
# #12 replaced this digest with sha256:8201530… — which reports go1.27.2 but
# is Debian trixie, so `apk` does not exist and every image build fails at
# this line with `apk: not found`. The digest was therefore reverted to the
# Alpine variant of the SAME Go version.
#
# `golang:1.27.2-alpine` still resolves to exactly this digest, so there is
# no newer Alpine 1.27.2 build to move to: a future bump must re-verify the
# base OS (cat /etc/os-release) and not just the Go version. This is also
# why the release path is not merely a version bump.
FROM golang@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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


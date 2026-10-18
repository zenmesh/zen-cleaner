# Build Requirements

## Dependencies

zen-cleaner has no external private dependencies. All first-party implementation is maintained locally in `internal/` packages.

## Build

```bash
# Build the controller
go build -o bin/zen-cleaner ./cmd/zen-cleaner

# Run tests
go test ./...

# Build Docker image
docker build -t zenmesh/zen-cleaner:latest .
```

## Local Development

No special requirements - standard Go tooling works out of the box.
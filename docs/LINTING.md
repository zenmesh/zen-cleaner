# Linting policy

This repository uses [golangci-lint](https://golangci-lint.run/) **v2** with a configuration tuned for **green CI** and meaningful signal for contributors.

## Running locally

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
golangci-lint run --timeout=5m
```

Use **Go 1.26+** (matches `go.mod`).

## What is enforced today

CI runs **`golangci-lint`** with correctness linters (`govet`, `staticcheck`, `errcheck`, `errorlint`, …), **`gosec`**, formatters, and style rules including **`goconst`**, **`godot`**, **`dupl`**, **`revive`**, **`err113`**, **`mnd`**, and **`gocritic`**. See `.golangci.yml` for the exact list and exclusions.

**Test-only carve-outs:** **`_test.go`** files skip **`godot`**, **`err113`**, **`dupl`**, and **`goconst`** so tests stay readable. Production packages (including **`internal/logging`** and **`pkg/controller/testing`**) are fully linted.

Environment validation in **`internal/config`** exports sentinel errors (for example **`ErrEnvRequired`**) so callers can use **`errors.Is`**; messages still include the variable name via **`fmt.Errorf`** wrapping.

## Tuning

**`goconst`** uses **`min-occurrences: 6`**. Shared Kubernetes literals for controller tests live in **`pkg/controller/testing/k8s_literals_test.go`**.

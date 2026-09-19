# Testing Guide

This document describes the testing infrastructure and how to run tests for zen-cleaner.

## Overview

zen-cleaner has three types of tests:

1. **Unit Tests**: Fast, isolated tests for individual components
2. **Integration Tests**: Tests that verify component interactions using fake clients
3. **E2E Tests**: End-to-end tests that require a real Kubernetes cluster

## Unit Tests

Unit tests are located in `pkg/*/*_test.go` files and test individual functions and components in isolation.

### Running Unit Tests

```bash
# Run all unit tests
make test-unit

# Run tests for a specific package
go test -v ./pkg/controller/...

# Run tests with race detection
go test -race ./pkg/...

# Run tests with coverage (same packages as CI/Makefile; excludes generated API types and internal/logging)
make coverage

# Or manually:
make test-unit
go tool cover -html=coverage.out -o coverage.html
```

### Coverage Requirements

- **Minimum**: 65% code coverage (`make coverage` fails below this threshold)
- **Target**: >80% overall coverage
- **Critical paths**: >85% coverage per package

Overall coverage is measured across `pkg/*` and `internal/*` test packages (see `COVERAGE_PKGS` in the `Makefile`). **CI enforces the 65% minimum** via `make coverage` on every PR (same command you run locally).

Integration and E2E tests add confidence beyond these unit-test metrics—especially for `pkg/controller`, where client-go makes exhaustive unit testing expensive.

Regenerate the figures in this document with:

```bash
make coverage
go tool cover -func=coverage.out | tail -1
```

### Current Coverage Status

**Overall Coverage**: **65.4%** ✅ (meets 65% minimum; last verified with `make test-unit`)

| Package | Coverage | Status | Notes |
|---------|----------|--------|-------|
| `pkg/config` | 95.0% | ✅ Excellent | |
| `pkg/errors` | 100.0% | ✅ Perfect | |
| `pkg/validation` | 87.6% | ✅ Excellent | |
| `pkg/webhook` | 80.3% | ✅ Good | |
| `internal/config` | 93.9% | ✅ Excellent | |
| `internal/backoff` | 100.0% | ✅ Perfect | |
| `internal/ratelimiter` | 100.0% | ✅ Perfect | |
| `internal/events` | 85.0% | ✅ Good | |
| `internal/errors` | 89.2% | ✅ Excellent | |
| `internal/health` | 81.6% | ✅ Good | |
| `internal/ttl` | 81.2% | ✅ Good | |
| `internal/election` | 71.2% | ✅ Good | |
| `pkg/controller` | 51.4% | ⚠️ Low unit % | See [controller coverage note](#pkgcontroller-unit-coverage) below |

#### `pkg/controller` unit coverage

**51.4% unit coverage on `pkg/controller` is expected and acceptable** for a Kubernetes controller—not a sign of missing quality gates.

- **Why the number is lower**: Reconcilers, informers, and deletion paths sit on top of client-go and the dynamic client. Meaningful unit tests require large fake clients and table fixtures; much of the “real” behavior only shows up with a live API server.
- **What we rely on instead**: Integration tests (`test/integration/`) exercise policy lifecycle with fake clients. **E2E / kind** (`make e2e-kind`, `make test-e2e` with `test/e2e/setup_kind.sh`) deploy the controller and validate critical paths on a real cluster—policy apply, reconciliation, and cleanup.
- **What we still improve**: Table-driven unit tests with fakes remain welcome for pure logic (TTL evaluation, selectors, status updates). We do **not** require `pkg/controller` to match the 65% **overall** gate on its own; the **repo-wide** 65% minimum (enforced in CI) is the merge bar.

**Areas for future improvement** (optional, not blockers for launch):

1. **`pkg/controller`** — more focused unit tests around evaluation helpers and status updates
2. **Per-package stretch goals** — 65%+ on controller, 80%+ overall, when cost-effective

**Test Strategy**:
- ✅ Unit tests: Strong coverage for config, validation, errors, webhooks, and shared `internal/*` helpers; **65% overall enforced in CI**
- ✅ Integration tests: Controller lifecycle and policy interactions (fake clients)
- ✅ E2E / kind: Real cluster paths for reconciliation and cleanup behavior
- ⚠️ `pkg/controller` unit %: Low by design; do not open drive-by “fix coverage” PRs without reading this section

## Integration Tests

Integration tests are located in `test/integration/` and test component interactions using fake Kubernetes clients.

### Test Coverage

Integration tests cover:

- ✅ Controller startup and shutdown
- ✅ Policy CRUD operations
- ✅ Policy deletion and cleanup
- ✅ Informer cleanup on policy deletion
- ✅ Rate limiter behavior
- ✅ Error recovery scenarios
- ✅ Multiple policies handling

### Running Integration Tests

```bash
# Run all integration tests
make test-integration

# Run specific integration test
go test -v ./test/integration/... -run TestCleanerController_PolicyDeletion
```

### Writing Integration Tests

Integration tests use fake Kubernetes clients (`dynamicfake`, `fake.Clientset`) to simulate Kubernetes API interactions without requiring a real cluster.

Example:

```go
func TestCleanerController_PolicyDeletion(t *testing.T) {
    scheme := runtime.NewScheme()
    if err := v1alpha1.AddToScheme(scheme); err != nil {
        t.Fatalf("Failed to add scheme: %v", err)
    }
    
    dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
    // ... test implementation
}
```

## E2E Tests

E2E tests are located in `test/e2e/` and require a real Kubernetes cluster.

### Prerequisites

- Kubernetes cluster (kind, k3s, minikube, or any cluster)
- kubectl configured to access the cluster
- Zen Cleaner Controller CRDs installed
- Zen Cleaner Controller deployed

### Quick Start with kind

```bash
# Setup E2E test cluster (creates kind cluster and deploys controller)
make test-e2e-setup

# Export kubeconfig
export KUBECONFIG=$(cd test/e2e && ./setup_kind.sh kubeconfig)

# Run E2E tests
make test-e2e

# Cleanup
make test-e2e-cleanup
```

### Manual Setup

1. **Install CRDs**:
   ```bash
   kubectl apply -f deploy/crds/
   ```

2. **Deploy Controller**:
   ```bash
   kubectl apply -f deploy/manifests/
   ```

3. **Run Tests**:
   ```bash
   go test -v -tags=e2e ./test/e2e/...
   ```

### E2E Test Scenarios

- ✅ Policy creation and validation
- ✅ Policy deletion and cleanup
- ✅ Resource deletion (with dry-run)
- ✅ Multiple policies handling

### E2E Test Infrastructure

The `test/e2e/setup_kind.sh` script provides:

- **kind cluster creation** with proper port mappings
- **CRD installation**
- **Controller deployment**
- **Image building and loading**

Usage:

```bash
# Create cluster and deploy controller
./test/e2e/setup_kind.sh create

# Delete cluster
./test/e2e/setup_kind.sh delete

# Get kubeconfig path
./test/e2e/setup_kind.sh kubeconfig
```

## Load Testing

Load tests verify controller performance under various workloads.

### Running Load Tests

```bash
# Basic load test (1000 ConfigMaps, 60s TTL)
make test-load

# Custom load test
cd test/load
./load_test.sh --count 5000 --resource ConfigMap --ttl 120

# Automated load tests with performance regression detection
make test-load-automated
```

### Load Test Scripts

1. **`load_test.sh`**: Basic load test script
   - Creates test resources
   - Waits for TTL expiration
   - Verifies deletion
   - Checks metrics

2. **`load_test_automated.sh`**: Automated load testing with regression detection
   - Runs multiple test scenarios
   - Collects performance metrics
   - Compares with baseline
   - Detects performance regressions

### Load Test Scenarios

Default scenarios in automated tests:

- 100 ConfigMaps, 60s TTL
- 1,000 ConfigMaps, 60s TTL
- 5,000 ConfigMaps, 60s TTL
- 100 Pods, 60s TTL
- 1,000 Pods, 60s TTL

### Performance Baseline

To create a performance baseline:

```bash
cd test/load
./load_test_automated.sh --all --baseline
```

This saves results to `results/baseline.json`.

### Performance Regression Detection

Compare current results with baseline:

```bash
cd test/load
./load_test_automated.sh --all --compare
```

The script will:
- Compare deletion durations
- Alert if performance degrades >20%
- Save results for analysis

### Load Test Options

```bash
# Basic load test options
./load_test.sh [OPTIONS]
  -n, --namespace NAME       Zen Cleaner Controller namespace (default: zen-cleaner-system)
  -t, --test-ns NAME         Test namespace (default: gc-load-test)
  -c, --count COUNT          Number of resources (default: 1000)
  -r, --resource TYPE        Resource type (default: ConfigMap)
  -s, --ttl SECONDS          TTL in seconds (default: 60)
  --no-cleanup               Don't cleanup test resources

# Automated load test options
./load_test_automated.sh [OPTIONS]
  --scenario COUNT:TYPE:TTL  Run specific scenario
  --all                      Run all scenarios (default)
  --baseline                 Create baseline from current results
  --compare                  Compare with baseline
  --results-dir DIR          Results directory (default: ./results)
```

## CI/CD Integration

### GitHub Actions

Tests run automatically on:

- **Push** to `main` or `develop` branches
- **Pull Requests** to `main` or `develop` branches

CI Pipeline includes:

- ✅ Unit tests with coverage check
- ✅ Integration tests
- ✅ Build verification
- ✅ Security scanning
- ⚠️ E2E tests (optional, can be run manually)

### Running Tests Locally

```bash
# Run all tests (unit + integration)
make test

# Run with coverage
make coverage

# Run E2E tests (requires cluster)
make test-e2e

# Run load tests
make test-load
```

## Test Best Practices

### Unit Tests

- ✅ Test one thing at a time
- ✅ Use table-driven tests for multiple scenarios
- ✅ Mock external dependencies
- ✅ Test error cases
- ✅ Test edge cases

### Integration Tests

- ✅ Use fake clients for isolation
- ✅ Test component interactions
- ✅ Verify cleanup and resource management
- ✅ Test error recovery

### E2E Tests

- ✅ Use dry-run mode when possible
- ✅ Clean up test resources
- ✅ Use unique test namespaces
- ✅ Handle cluster unavailability gracefully
- ✅ Skip tests if prerequisites not met

### Load Tests

- ✅ Use appropriate TTL values for testing
- ✅ Monitor resource usage
- ✅ Collect metrics
- ✅ Compare with baseline
- ✅ Clean up test resources

## Troubleshooting

### Integration Tests Fail

**Issue**: Tests fail with "scheme not found" errors

**Solution**: Ensure `v1alpha1.AddToScheme(scheme)` is called before creating fake clients.

### E2E Tests Skip

**Issue**: Tests skip with "CRD not installed"

**Solution**: 
```bash
kubectl apply -f deploy/crds/
kubectl wait --for condition=established crd/zencleanerpolicies.cleaner.zen-mesh.io
```

### Load Tests Timeout

**Issue**: Resources not deleted within timeout

**Solution**:
- Check controller logs: `kubectl logs -n zen-cleaner-system -l app=zen-cleaner`
- Verify policy is active: `kubectl get zencleanerpolicies`
- Check metrics: `kubectl port-forward -n zen-cleaner-system svc/zen-cleaner-metrics 8080:8080`

### kind Cluster Issues

**Issue**: Cannot create kind cluster

**Solution**:
- Ensure Docker is running
- Check available resources: `docker system df`
- Try deleting existing cluster: `kind delete cluster --name zen-cleaner-e2e`

## Expected Test Results

### Unit Tests

```
ok  	github.com/zenmesh/zen-cleaner/internal/backoff	…	coverage: 100.0% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/config	…	coverage: 93.9% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/election	…	coverage: 71.2% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/errors	…	coverage: 89.2% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/events	…	coverage: 85.0% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/health	…	coverage: 81.6% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/ratelimiter	…	coverage: 100.0% of statements
ok  	github.com/zenmesh/zen-cleaner/internal/ttl	…	coverage: 81.2% of statements
ok  	github.com/zenmesh/zen-cleaner/pkg/config	…	coverage: 95.0% of statements
ok  	github.com/zenmesh/zen-cleaner/pkg/controller	…	coverage: 51.4% of statements
ok  	github.com/zenmesh/zen-cleaner/pkg/errors	…	coverage: 100.0% of statements
ok  	github.com/zenmesh/zen-cleaner/pkg/validation	…	coverage: 87.6% of statements
ok  	github.com/zenmesh/zen-cleaner/pkg/webhook	…	coverage: 80.3% of statements
total:										(statements)					65.4%
```

**Expected**: All tests pass; `go tool cover -func=coverage.out | tail -1` reports ≥65% total.

### E2E Tests

```
=== RUN   TestE2E_CleanerController
--- PASS: TestE2E_CleanerController (5.03s)
=== RUN   TestE2E_PolicyDeletion
--- PASS: TestE2E_PolicyDeletion (4.01s)
=== RUN   TestE2E_ResourceDeletion
--- PASS: TestE2E_ResourceDeletion (15.03s)
PASS
ok  	github.com/zenmesh/zen-cleaner/test/e2e	24.077s
```

**Expected**: 3 tests pass:
- `TestE2E_CleanerController` - Basic CRUD operations
- `TestE2E_PolicyDeletion` - Policy deletion behavior
- `TestE2E_ResourceDeletion` - Resource cleanup verification

### Running Tests for CI Verification

To verify your changes match expected results:

```bash
# Unit tests
go test ./... -cover

# E2E tests (requires kind cluster)
kind create cluster --name zen-cleaner-test
kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
KUBECONFIG=$(kind get kubeconfig --name zen-cleaner-test) go test -tags=e2e ./test/e2e/...
kind delete cluster --name zen-cleaner-test
```

## Performance Benchmarks

See [BENCHMARKS.md](BENCHMARKS.md) for detailed performance benchmarks and test results.

## References

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [kind Documentation](https://kind.sigs.k8s.io/)
- [Kubernetes Testing Guide](https://kubernetes.io/docs/concepts/cluster-administration/testing/)


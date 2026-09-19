# Contributing to zen-cleaner

Thank you for your interest in contributing to zen-cleaner! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- **Go**: 1.26 or later ([Download](https://golang.org/dl/)) — matches `go.mod` / CI
- **kubectl**: Configured to access a Kubernetes cluster ([Install](https://kubernetes.io/docs/tasks/tools/))
- **Docker**: For building images ([Install](https://docs.docker.com/get-docker/))
- **Make**: For running common tasks ([Install](https://www.gnu.org/software/make/))
- **kind** (optional): For local E2E testing ([Install](https://kind.sigs.k8s.io/))
- **kubebuilder** (optional): For CRD development ([Install](https://kubebuilder.io/))

### Getting Started

1. **Fork and clone the repository:**
   ```bash
   git clone https://github.com/zen-mesh/zen-cleaner.git
   cd zen-cleaner
   ```

2. **Verify Go installation:**
   ```bash
   go version  # Should be 1.26+
   ```

3. **Install dependencies:**
   ```bash
   go mod download
   ```

4. **Install development tools:**
   ```bash
   # Install golangci-lint v2 (required — see docs/LINTING.md)
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
   
   # Install other tools (if Makefile has install-tools target)
   make install-tools
   ```

5. **Install pre-commit hooks (recommended):**
   ```bash
   pip install pre-commit
   pre-commit install
   ```

   This will run checks automatically before each commit, catching issues early.

### Local Development Environment

#### Option 1: Local Development with kind

1. **Create kind cluster:**
   ```bash
   cd test/e2e
   ./setup_kind.sh create
   export KUBECONFIG=$(./setup_kind.sh kubeconfig)
   ```

2. **Build and deploy controller:**
   ```bash
   # Build image
   docker build -t zenmesh/zen-cleaner:dev .
   kind load docker-image zenmesh/zen-cleaner:dev --name zen-cleaner-e2e
   
   # Deploy
   kubectl apply -f deploy/manifests/
   kubectl set image deployment/zen-cleaner zen-cleaner=zenmesh/zen-cleaner:dev -n zen-cleaner-system
   ```

3. **Run controller locally (for faster iteration):**
   ```bash
   # In one terminal: Run controller locally
   go run cmd/zen-cleaner/main.go \
     --kubeconfig=$KUBECONFIG \
     --metrics-addr=:8080
   
   # In another terminal: Test with policies
   kubectl apply -f examples/temp-configmap-cleanup.yaml
   ```

#### Option 2: Remote Cluster Development

1. **Configure kubectl:**
   ```bash
   kubectl config use-context <your-cluster-context>
   kubectl cluster-info
   ```

2. **Deploy to cluster:**
   ```bash
   # Build and push image
   docker build -t <your-registry>/zen-cleaner:dev .
   docker push <your-registry>/zen-cleaner:dev
   
   # Deploy
   kubectl apply -f deploy/manifests/
   kubectl set image deployment/zen-cleaner zen-cleaner=<your-registry>/zen-cleaner:dev -n zen-cleaner-system
   ```

#### Option 3: Unit Test Only (No Cluster)

For most development, you can work with unit tests only:

```bash
# Run unit tests
make test-unit

# Run specific test
go test -v ./pkg/controller/... -run TestCleanerController_StartStop
```

### IDE Setup

#### VS Code

1. **Install Go extension:**
   - Install "Go" extension by Go Team at Google

2. **Configure settings:**
   ```json
   {
     "go.testFlags": ["-v", "-race"],
     "go.lintTool": "golangci-lint",
     "go.lintFlags": ["--fast"]
   }
   ```

3. **Install Go tools:**
   - Open Command Palette (Ctrl+Shift+P)
   - Run "Go: Install/Update Tools"
   - Select all tools

#### GoLand / IntelliJ IDEA

1. **Configure Go SDK:**
   - File → Settings → Go → GOROOT
   - Set to your Go installation path

2. **Enable Go modules:**
   - File → Settings → Go → Go Modules
   - Enable "Enable Go modules integration"

### Development Workflow

1. **Create feature branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make changes and test:**
   ```bash
   # Run tests
   make test-unit
   
   # Check formatting
   make fmt
   
   # Run linter
   make lint
   ```

3. **Commit changes:**
   ```bash
   git add .
   git commit -m "feat: add your feature"
   # Pre-commit hooks run automatically
   ```

4. **Push and create PR:**
   ```bash
   git push origin feature/your-feature-name
   # Create PR on GitHub
   ```

### Debugging

#### Debug Controller Locally

```bash
# Run with debug logging
go run cmd/zen-cleaner/main.go \
  --kubeconfig=$KUBECONFIG \
  --v=5  # Verbose logging
```

#### Debug in VS Code

1. Create `.vscode/launch.json`:
   ```json
   {
     "version": "0.2.0",
     "configurations": [
       {
         "name": "Launch Controller",
         "type": "go",
         "request": "launch",
         "mode": "auto",
         "program": "${workspaceFolder}/cmd/zen-cleaner",
         "env": {
           "KUBECONFIG": "${env:HOME}/.kube/config"
         },
         "args": ["--v=5"]
       }
     ]
   }
   ```

2. Set breakpoints and press F5

#### Debug in Cluster

```bash
# Port-forward to controller
kubectl port-forward -n zen-cleaner-system deployment/zen-cleaner 8080:8080

# View logs
kubectl logs -n zen-cleaner-system -l app=zen-cleaner -f

# Describe pod
kubectl describe pod -n zen-cleaner-system -l app=zen-cleaner
```

### Common Development Tasks

```bash
# Run all tests
make test

# Run specific test package
go test -v ./pkg/controller/...

# Run tests with coverage
make coverage

# Format code
make fmt

# Lint code
make lint

# Build binary
make build

# Build Docker image
make build-image

# Run integration tests
make test-integration

# Run E2E tests (requires cluster)
make test-e2e

# Clean build artifacts
make clean
```

### Project Structure

```
zen-cleaner/
├── cmd/              # Application entry points
│   └── zen-cleaner/
├── pkg/              # Library code
│   ├── api/          # API types
│   ├── controller/  # Controller logic
│   ├── validation/   # Validation logic
│   └── webhook/      # Webhook server
├── deploy/           # Deployment manifests
├── test/             # Test code
│   ├── integration/  # Integration tests
│   ├── e2e/          # E2E tests
│   └── load/         # Load tests
├── docs/             # Documentation
├── examples/         # Example policies
└── charts/           # Helm charts
```

### Tips for Faster Development

1. **Use unit tests** for most development (faster than integration tests)
2. **Run specific tests** instead of full test suite during development
3. **Use dry-run mode** when testing policies
4. **Enable verbose logging** (`--v=5`) for debugging
5. **Use kind** for local E2E testing (faster than remote cluster)

## Pre-commit Hooks

We use pre-commit hooks to ensure code quality. The hooks run automatically on commit and check:

- Code formatting (gofmt)
- Go vet
- YAML/JSON syntax
- License headers
- Large files
- Merge conflicts
- And more...

To run hooks manually:
```bash
pre-commit run --all-files
```

## Development Workflow

### Making Changes

1. **Create a branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**

3. **Run tests:**
   ```bash
   make test-unit
   ```

4. **Check code quality:**
   ```bash
   make verify
   make lint
   ```

5. **Commit your changes:**
   ```bash
   git add .
   git commit -m "feat: add your feature"
   ```
   
   Pre-commit hooks will run automatically. If they fail, fix the issues and commit again.

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Test changes
- `chore:` - Build/tooling changes

Examples:
```
feat: add support for custom TTL calculations
fix: handle nil pointer in policy evaluation
docs: update architecture documentation
```

### Testing

- **Unit tests:**
  ```bash
  make test-unit
  ```

- **Integration tests:**
  ```bash
  make test-integration
  ```

- **E2E tests (requires cluster):**
  ```bash
  make test-e2e
  ```

- **Coverage:**
  ```bash
  make coverage
  ```

### Building

- **Development build:**
  ```bash
  make build
  ```

- **Optimized release build:**
  ```bash
  make build-release
  ```

- **Docker image:**
  ```bash
  make build-image
  ```

## Code Style

- Follow Go standard formatting (`gofmt`)
- Run `make fmt` before committing
- Follow `.golangci.yml` and **[docs/LINTING.md](docs/LINTING.md)** (scope and known debt)
- Add comments for exported functions/types
- Keep functions focused and small

## Pull Requests

1. **Ensure your branch is up to date:**
   ```bash
   git checkout main
   git pull origin main
   git checkout your-branch
   git rebase main
   ```

2. **Push your branch:**
   ```bash
   git push origin your-branch
   ```

3. **Create a PR on GitHub**

4. **Ensure CI passes** - All checks must pass before merge

### PR Checklist

- [ ] Code follows style guidelines
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Commit messages follow conventions
- [ ] Pre-commit hooks pass
- [ ] CI checks pass

## Reporting Issues

When reporting bugs or requesting features:

1. Check existing issues first
2. Use the issue templates
3. Provide:
   - Clear description
   - Steps to reproduce (for bugs)
   - Expected vs actual behavior
   - Environment details (Kubernetes version, etc.)

## Code Review

- Be respectful and constructive
- Focus on the code, not the person
- Ask questions if something is unclear
- Suggest improvements, don't just point out problems

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

- Check the [documentation](docs/)
- Open an issue for questions
- Join our community discussions

Thank you for contributing! 🎉

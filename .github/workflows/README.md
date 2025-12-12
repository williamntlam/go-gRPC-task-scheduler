# GitHub Actions CI/CD Pipeline

This directory contains GitHub Actions workflows for continuous integration and deployment.

## Overview

All CI/CD is handled through GitHub Actions. The workflows run automatically when you push code or create pull requests.

## Workflows

### 1. CI (`ci.yml`)
Runs on every push and pull request to `main` and `develop` branches.

**Jobs:**
- **Lint**: Code formatting and static analysis
- **Unit Tests**: Fast unit tests with race detection
- **Integration Tests**: Full integration tests with CockroachDB and Redis
- **Build**: Docker image builds
- **TLS Tests**: Certificate generation and validation
- **System Test**: Full system test with Docker Compose

### 2. CD (`cd.yml`)
Runs on pushes to `main` and when tags are created.

**Jobs:**
- **Build and Push**: Builds and pushes Docker images to GitHub Container Registry
- **Deploy Staging**: Deploys to staging environment
- **Deploy Production**: Deploys to production (only on version tags)

### 3. Release (`release.yml`)
Creates GitHub releases when version tags are pushed.

**Features:**
- Runs full test suite
- Generates changelog from git commits
- Creates release with release notes

### 4. Security (`security.yml`)
Security scanning for vulnerabilities.

**Scans:**
- Go code with Gosec
- Docker images with Trivy
- Runs weekly and on push/PR

### 5. Dependabot (`dependabot.yml`)
Auto-merges Dependabot PRs for patch and minor updates.

## Setup

### Required Secrets

For deployment, you may need to configure:

- `GITHUB_TOKEN` - Automatically provided by GitHub Actions
- Container registry credentials (if using external registry)
- Deployment environment secrets (Kubernetes, cloud provider, etc.)

### Environment Variables

The workflows use environment variables defined in the workflow files. For local testing:

```bash
export TEST_COCKROACHDB_HOST=localhost
export TEST_COCKROACHDB_PORT=26257
export TEST_REDIS_HOST=localhost
export TEST_REDIS_PORT=6379
```

## Local Testing

You can test the CI pipeline locally using [act](https://github.com/nektos/act):

```bash
# Install act
brew install act  # macOS
# or
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Run CI workflow
act push

# Run specific job
act -j unit-tests
```

## Customization

### Adding New Tests

Add test steps to the appropriate job in `ci.yml`:

```yaml
- name: Run custom tests
  run: go test ./path/to/tests -v
```

### Changing Deployment

Modify the `deploy-staging` and `deploy-production` jobs in `cd.yml` with your deployment commands.

### Adding Environments

Add new environments to the `cd.yml` workflow:

```yaml
deploy-custom:
  name: Deploy to Custom
  runs-on: ubuntu-latest
  environment:
    name: custom
  steps:
    - name: Deploy
      run: echo "Deploying..."
```

## Monitoring

- View workflow runs: https://github.com/YOUR_REPO/actions
- Check test results in the Actions tab
- Security alerts appear in the Security tab

## Troubleshooting

### Tests Failing

1. Check service health in integration tests
2. Verify environment variables are set
3. Check Docker service availability

### Build Failures

1. Verify Dockerfile syntax
2. Check for missing dependencies
3. Review build logs for specific errors

### Deployment Issues

1. Verify secrets are configured
2. Check deployment environment permissions
3. Review deployment logs

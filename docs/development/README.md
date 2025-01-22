# Development Guide

## Prerequisites

The Linode Cloud Controller Manager development requires:
- A fairly up-to-date GNU tools environment
- Go 1.23 or higher

### Setting Up Development Environment

#### Option 1: Using Devbox (Recommended)
The simplest way to set up your development environment is using [Devbox](https://www.jetpack.io/devbox/):

1. Install Devbox by following the instructions at [jetpack.io/devbox/docs/installing_devbox/](https://www.jetpack.io/devbox/docs/installing_devbox/)

2. Start the development environment:
```bash
devbox shell
```

This will automatically set up all required dependencies and tools for development.

#### Option 2: Manual Setup

1. If you haven't set up a Go development environment, follow [these instructions](https://golang.org/doc/install) to install Go.

On macOS, you can use Homebrew:
```bash
brew install golang
```

## Getting Started

### Download Source
```bash
go get github.com/linode/linode-cloud-controller-manager
cd $(go env GOPATH)/src/github.com/linode/linode-cloud-controller-manager
```

### Building the Project

#### Build Binary
Use the following Make targets to build and run a local binary:

```bash
# Build the binary
make build

# Run the binary
make run

# You can also run the binary directly to pass additional args
dist/linode-cloud-controller-manager
```

#### Building Docker Images
To build and push a Docker image:

```bash
# Set the repo/image:tag with the TAG environment variable
# Then run the docker-build make target
IMG=linode/linode-cloud-controller-manager:canary make docker-build

# Push Image
IMG=linode/linode-cloud-controller-manager:canary make docker-push
```

To run the Docker image:
```bash
docker run -ti linode/linode-cloud-controller-manager:canary
```

### Managing Dependencies
The Linode Cloud Controller Manager uses [Go Modules](https://blog.golang.org/using-go-modules) to manage dependencies.

To update or add dependencies:
```bash
go mod tidy
```

## Development Guidelines

### Code Quality Standards
- Write correct, up-to-date, bug-free, fully functional, secure, and efficient code
- Use the latest stable version of Go
- Follow Go idioms and best practices
- Implement proper error handling with custom error types when beneficial
- Include comprehensive input validation
- Utilize built-in language features for performance optimization
- Follow relevant design patterns and principles
- Leave NO todos, placeholders, or incomplete implementations

### Code Structure
- Include necessary imports and declarations
- Implement proper logging using appropriate logging mechanisms
- Consider implementing middleware or interceptors for cross-cutting concerns
- Structure code in a modular and maintainable way
- Use appropriate naming conventions and code organization

### Security & Performance
- Implement security best practices
- Consider rate limiting when appropriate
- Include authentication/authorization where needed
- Optimize for performance while maintaining readability
- Consider scalability in design decisions

### Documentation & Testing
- Provide brief comments for complex logic or language-specific idioms
- Include clear documentation for public interfaces
- Write tests using appropriate testing frameworks
- Document any assumptions or limitations

### Pull Request Process
1. Ensure your code follows the project's coding standards
2. Update documentation as needed
3. Add or update tests as appropriate
4. Make sure all tests pass locally
5. Submit the PR with a clear description of the changes

## Getting Help
For development related questions or discussions, join us in #linode on the [Kubernetes Slack](https://kubernetes.slack.com/messages/CD4B15LUR/details/). 
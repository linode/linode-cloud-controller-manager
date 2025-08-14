# Linode Cloud Controller Manager - AI Coding Assistant Instructions

## Project Overview
This is the Kubernetes Cloud Controller Manager for Linode infrastructure. It integrates Kubernetes clusters with Linode's cloud services, managing load balancers (NodeBalancers), firewalls (Cloud Firewalls),  nodes, networking routes, and IP allocation.

## Architecture & Core Components

### Cloud Provider Interface
- `cloud/linode/cloud.go` - Main cloud provider implementation following Kubernetes CloudProvider interface
- Implements 4 controllers: Node, Service, Route, and NodeIPAM controllers
- All controllers run as goroutines initialized in `Initialize()` method

### Controllers Structure
- **Node Controller** (`node_controller.go`) - Manages node metadata, addresses, and lifecycle
- **Service Controller** (`service_controller.go`) - Handles service deletions and cleanup
- **Route Controller** (`route_controller.go`) - Manages VPC routes and pod networking
- **NodeIPAM Controller** (`nodeipamcontroller.go`) - Allocates pod CIDRs to nodes

### Load Balancer Types
Two distinct implementations:
1. **NodeBalancer** (`loadbalancers.go`) - Traditional Linode NodeBalancers
2. **Cilium BGP** (`cilium_loadbalancers.go`) - BGP-based IP sharing

## Development Patterns

### Client Abstraction
- `cloud/linode/client/client.go` - Interface wrapping Linode API
- `client_with_metrics.go` - Prometheus metrics decorator using generated code
- Use `//go:generate` with gowrap for automatic client instrumentation

### Testing Strategy
- Extensive use of gomock for Linode API client mocking
- `fake_linode_test.go` - In-memory fake implementation for integration tests
- Table-driven tests pattern throughout codebase
- Mock generation: `//go:generate go run github.com/golang/mock/mockgen`

### Configuration Management
- `Options` global struct in `cloud.go` for CLI flags and environment variables
- Annotation-driven service configuration via `cloud/annotations/annotations.go`
- All annotations prefixed with `service.beta.kubernetes.io/linode-loadbalancer-`

## Key Development Workflows

### Building & Testing
```bash
# Use devbox for development environment
devbox shell

# Build binary
make build

# Run tests with coverage
make test-coverage

# Generate mocks (run after changing client interface)
make mockgen
```

### Adding New Annotations
1. Add constant to `cloud/annotations/annotations.go`
2. Update `docs/configuration/annotations.md` documentation
3. Implement parsing logic in relevant controller
4. Add validation and tests

### Working with Load Balancers
- Check `loadbalancers.go` for NodeBalancer implementation patterns
- Use `portConfigAnnotation` struct for JSON port configurations
- VPC integration uses `getVPCCreateOptions()` for subnet selection
- Health check configuration follows annotation-driven pattern

## Project-Specific Conventions

### Error Handling
- Custom error types: `invalidProviderIDError`, `lbNotFoundError`
- Use `ignoreLinodeAPIError()` helper for handling expected API errors
- Sentry integration for error reporting in production

### Logging Patterns
- Use `klog` package throughout (Kubernetes standard)
- Log levels: Info for normal operations, Error for failures, V(3) for debug
- Include context like service names, node names in log messages

### Resource Management
- Provider ID format: `linode://12345` (instance ID)
- Node caching with TTL in `k8sNodeCache` for performance
- Workqueue pattern for asynchronous controller processing

### Deployment
- DaemonSet deployment on control plane nodes only
- Uses `hostNetwork: true` for API access
- Requires `LINODE_API_TOKEN` and `LINODE_REGION` environment variables
- RBAC permissions defined in `deploy/ccm-linode-template.yaml`

## Integration Points

### Kubernetes APIs
- Implements standard CloudProvider interface from `k8s.io/cloud-provider`
- Node informers for real-time node updates
- Service informers for load balancer lifecycle management

### Linode API Integration
- Wrapped via `linodego` client library
- Rate limiting and retry logic built into client wrapper
- Prometheus metrics automatically generated for all API calls

### Cilium Integration
- BGP mode requires Cilium CNI with BGP peering
- Creates CiliumLoadBalancerIPPool resources for IP management
- Uses node selectors for BGP-enabled nodes

## Common Gotchas
- NodeBalancer backends require VPC configuration when `Options.VPCNames` is set
- Node exclusion annotation: `node.k8s.linode.com/exclude-from-nb`
- Port configurations must be valid JSON in annotations
- Health check types depend on protocol (UDP vs TCP/HTTP)
- Instance cache is global and shared across controllers

## Documentation
- Keep `docs/configuration/annotations.md` synchronized with code changes
- All new features require documentation in `docs/` directory
- Examples in `docs/examples/` should reflect real-world usage patterns

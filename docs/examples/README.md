# Examples

This section provides working examples of common CCM configurations. Each example includes a complete service and deployment configuration.

## Available Examples

1. **[Basic Services](basic.md)**
   - HTTP LoadBalancer
   - HTTPS LoadBalancer with TLS termination
   - UDP LoadBalancer

2. **[Advanced Configuration](advanced.md)**
   - Custom Health Checks
   - Firewalled Services
   - Session Affinity
   - Shared IP Load-Balancing
   - Custom Node Selection

Note: To test UDP based NBs, one can use [test-server](https://github.com/rahulait/test-server) repo to run server using UDP protocol and then use the client commands in repo's readme to connect to the server.

For testing these examples, see the [test script](https://github.com/linode/linode-cloud-controller-manager/blob/master/examples/test.sh).

For more configuration options, see:
- [Service Annotations](../configuration/annotations.md)
- [LoadBalancer Configuration](../configuration/loadbalancer.md)
- [Firewall Configuration](../configuration/firewall.md)

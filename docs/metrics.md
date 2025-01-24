### Prometheus metrics

Cloud Controller Manager exposes metrics by default on port given by
`--secure-port` flag. The endpoint is protected from unauthenticated access by
default.  To allow unauthenticated clients (`system:anonymous`) access
Prometheus metrics, use `--authorization-always-allow-paths="/metrics"` command
line flag.

Linode API calls can be monitored using `ccm_linode_client_requests_total` metric.

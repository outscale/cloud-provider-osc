# Annotation

The supported Service annotations are:

| Annotation | Mutable | Description |
| --- | --- | --- |
| service.beta.kubernetes.io/osc-load-balancer-name | No | The load balancer name (will be truncated to 32 chars). |
| service.beta.kubernetes.io/osc-load-balancer-internal | No | Is it an internal LBU or a public one ? |
| service.beta.kubernetes.io/osc-load-balancer-subnet-id | No | The subnet in which to create the load balancer. |
| service.beta.kubernetes.io/osc-load-balancer-security-group | No | The main security group of the load balancer, if not set, a new SG will be created. |
| service.beta.kubernetes.io/osc-load-balancer-extra-security-groups | No | Additional security groups to be added |
| service.beta.kubernetes.io/osc-load-balancer-additional-resource-tags | No | A comma-separated list of key=value pairs which to be added as additional tags in the LBU. For example: `Key1=Val1,Key2=Val2,KeyNoVal1=,KeyNoVal2` |
| service.beta.kubernetes.io/osc-load-balancer-target-role | Yes | The role of backend server nodes (default: `worker`) |
| service.beta.kubernetes.io/osc-load-balancer-proxy-protocol | Yes | A comma-separated list of backend ports that will use proxy protocol. Set to value `*` to enable proxy protocol on all backends. |
| service.beta.kubernetes.io/osc-load-balancer-backend-protocol | Yes | The protocol to use to talk to backend pods. If `http` (default) or `https`, an HTTPS listener that terminates the connection and parses headers is created. If set to `ssl` or `tcp`, a "raw" SSL listener is used. If set to `http` and `osc-load-balancer-ssl-cert` is not used then a HTTP listener is used. |
| service.beta.kubernetes.io/osc-load-balancer-ssl-cert | Yes | The ORN of the certificate to use with a SSL/HTTPS listener. See https://docs.outscale.com/en/userguide/About-Server-Certificates-in-EIM.html for more info. |
| service.beta.kubernetes.io/osc-load-balancer-ssl-ports | A comma-separated list of ports that will use SSL/HTTPS. Defaults to '*' (all). |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-healthy-threshold | Yes | The number of successive successful health checks before marking a backend as healthy. |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-unhealthy-threshold | Yes | The number of unsuccessful health checks before marking a backend as unhealthy. |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-timeout | Yes | The timeout, in seconds, for health checks calls. |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-interval | Yes | The interval, in seconds,  between health checks. |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-port | Yes | The port number for health check requests (from 1 to 65535). |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-protocol | Yes | The protocol for health check requests (among `http`/`https`/`tcp`/`ssl`). |
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-path | Yes | The optional URL path for health check requests, only used with `http` or `https` health check protocols. |
| service.beta.kubernetes.io/osc-load-balancer-access-log-enabled | Yes | Enable access logs. |
| service.beta.kubernetes.io/osc-load-balancer-access-log-emit-interval | Yes | Access log emit interval. |
| service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-name | Yes | Access log OOS bucket name. |
| service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-prefix | Yes | Access log OOS bucket prefix. |
| service.beta.kubernetes.io/osc-load-balancer-connection-draining-enabled | Yes | Enable connection draining. |
| service.beta.kubernetes.io/osc-load-balancer-connection-draining-timeout | Yes | The connection draining timeout. |
| service.beta.kubernetes.io/osc-load-balancer-connection-idle-timeout | Yes | The idle connection timeout. |
| service.beta.kubernetes.io/load-balancer-source-ranges | Yes | The IP source ranges allowed to call the load balancer. |

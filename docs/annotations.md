# Annotation

The supported Service annotations are:

| Annotation | Default | Mutable | Description
| --- | --- | --- | ---
| service.beta.kubernetes.io/osc-load-balancer-name | n/a | No | The load balancer name (will be truncated to 32 chars) (one per instance).
| service.beta.kubernetes.io/osc-load-balancer-internal | `false` | No | Is it an internal LBU or a public one ?
| service.beta.kubernetes.io/osc-load-balancer-instances | n/a | No | The number of LBU instances to create.
| service.beta.kubernetes.io/osc-load-balancer-subregions | n/a | No | The subregions where to deploy the LBUs (one per instance).
| service.beta.kubernetes.io/osc-load-balancer-subnet-id | n/a | No | The subnet in which to create the load balancer (one per instance).
| service.beta.kubernetes.io/osc-load-balancer-ip-pool | n/a | No | The pool from which a public IP will be fetched (public IPs tagged with a `OscK8sIPPool:<pool name>` tag).
| service.beta.kubernetes.io/osc-load-balancer-ip-id | n/a | No | The ID of the public IP to use (one per instance).
| service.beta.kubernetes.io/osc-load-balancer-security-group | n/a | No | The main security group of the load balancer, if not set, a new SG will be created.
| service.beta.kubernetes.io/osc-load-balancer-extra-security-groups | n/a | No | Additional security groups to be added
| service.beta.kubernetes.io/osc-load-balancer-additional-resource-tags | n/a | No | A comma-separated list of key=value tags to be added to the LBU. For example: `Key1=Val1,Key2=Val2,KeyNoVal1=,KeyNoVal2`
| service.beta.kubernetes.io/osc-load-balancer-target-role | `worker` | Yes | The role of backend server nodes, used to search the target node security group.
| service.beta.kubernetes.io/osc-load-balancer-target-node-labels | n/a | Yes | A comma-separated list of key=value labels the backend server nodes need to have (by default, all nodes are added as LBU backends).
| service.beta.kubernetes.io/osc-load-balancer-proxy-protocol | n/a | Yes | A comma-separated list of backend ports that will use proxy protocol. Set to value `*` to enable proxy protocol on all backends.
| service.beta.kubernetes.io/osc-load-balancer-backend-protocol | n/a | Yes | The protocol to use to talk to backend pods. If `http` or `https`, an HTTPS listener that terminates the connection and parses headers is created. If set to `ssl` or `tcp`, a "raw" SSL listener is used. If set to `http` and `osc-load-balancer-ssl-cert` is not used then a HTTP listener is used.
| service.beta.kubernetes.io/osc-load-balancer-ssl-cert | n/a | Yes | The ORN of the certificate to use with a SSL/HTTPS listener. See https://docs.outscale.com/en/userguide/About-Server-Certificates-in-EIM.html for more info.
| service.beta.kubernetes.io/osc-load-balancer-ssl-ports | `*` | Yes | A comma-separated list of ports that will use SSL/HTTPS. Defaults to '*' (all).
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-healthy-threshold | `2` | Yes | The number of successive successful health checks before marking a backend as healthy.
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-unhealthy-threshold | `3` | Yes | The number of unsuccessful health checks before marking a backend as unhealthy.
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-timeout | `5` | Yes | The timeout, in seconds, for health checks calls.
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-interval | `10` | Yes | The interval, in seconds,  between health checks.
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-port | n/a | Yes | The port number for health check requests (from 1 to 65535).
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-protocol | n/a | Yes | The protocol for health check requests (among `http`/`https`/`tcp`/`ssl`).
| service.beta.kubernetes.io/osc-load-balancer-healthcheck-path | n/a | Yes | The optional URL path for health check requests, only used with `http` or `https` health check protocols.
| service.beta.kubernetes.io/osc-load-balancer-access-log-enabled | `false` | Yes | Enable access logs.
| service.beta.kubernetes.io/osc-load-balancer-access-log-emit-interval | n/a | Yes | Access log emit interval.
| service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-name | n/a | Yes | Access log OOS bucket name.
| service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-prefix | n/a | Yes | Access log OOS bucket prefix.
| service.beta.kubernetes.io/osc-load-balancer-connection-draining-enabled | n/a | Yes | Enable connection draining.
| service.beta.kubernetes.io/osc-load-balancer-connection-draining-timeout | n/a | Yes | The connection draining timeout.
| service.beta.kubernetes.io/osc-load-balancer-connection-idle-timeout | `60` | Yes | The idle connection timeout.
| service.beta.kubernetes.io/osc-load-balancer-ingress-address | `hostname` | Yes | Defines what information is returned in `status.loadBalancer.ingress`. `hostname` returns only the LBU hostname, `ip` only the LBU IP or `both` (sets both IP and hostname).
| service.beta.kubernetes.io/osc-load-balancer-ingress-ipmode | `Proxy` | Yes | Defines what information is returned in `status.loadBalancer.ingress.ipMode `: `Proxy` or `VIP` if `service.beta.kubernetes.io/osc-load-balancer-ingress-address` is set to `ip` or `both`.
| service.beta.kubernetes.io/load-balancer-source-ranges | n/a | Yes | The IP source ranges allowed to call the load balancer.

# Outscale Cloud Controller Manager (CCM)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/osc-cloud-controller-manager)](https://artifacthub.io/packages/search?repo=osc-cloud-controller-manager)
[![Project Graduated](https://docs.outscale.com/fr/userguide/_images/Project-Graduated-green.svg)](https://docs.outscale.com/en/userguide/Open-Source-Projects.html)

The Outscale Cloud Controller Manager (cloud-provider-osc) provides the interface between a Kubernetes cluster and 3DS Outscale service APIs. 
This project is necessary for operating the cluster.


More details on [cloud-controller role](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) in Kubernetes architecture.

# Features
- Node controller: provides Kubernetes details about nodes (Outscale Virtual Machines)
- Service controller: allows cluster user to expose Kubernetes Services using Outscale Load Balancer Unit (LBU) 

# Installation
See the [deployment documentation](../deploy/README.md)

# Upgrading to v1.0

The secret has now the same format as the CSI driver. You need to rename:
* `key_id` to `access_key`,
* `access_key` to `secret_key`.

All other entries can be deleted.

# Usage

Some example of services:
- [2048](../examples/2048)
- [simple-lb](../examples/simple-lb)
- [simple-internal](../examples/simple-internal)
- [advanced-lb](../examples/advanced-lb)

Services can be annotated to adapt behavior and configuration of Load Balancer Units.
Check [annotation documentation](../docs/annotations.md) for more details.

# Contributing

For new feature request or bug fixes, please [create an issue](https://github.com/outscale/cloud-provider-osc/issues).

If you want to dig into cloud-provider-osc, check [development documentation](development.md).

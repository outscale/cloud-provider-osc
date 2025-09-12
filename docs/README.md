# Outscale Cloud Controller Manager (CCM)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/osc-cloud-controller-manager)](https://artifacthub.io/packages/search?repo=osc-cloud-controller-manager)
[![Project Graduated](https://docs.outscale.com/fr/userguide/_images/Project-Graduated-green.svg)](https://docs.outscale.com/en/userguide/Open-Source-Projects.html)

The Outscale Cloud Controller Manager (cloud-provider-osc) provides the interface between a Kubernetes cluster and the OUTSCALE Cloud. 
This component is required to operate a cluster on the OUTSCALE Cloud.

More details on the [cloud-controller role](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) in a Kubernetes cluster.

# Features
- Node controller: provides information and status about nodes,
- Service controller: allows creation of LoadBalancer services, based on [Load Balancer Units (LBU)](https://docs.outscale.com/en/userguide/About-Load-Balancers.html). 

# Installation
See the [deployment documentation](../deploy/README.md)

# Upgrading to v1.0

Annotations have changed, but the old ones remain valid. You do not need to update your existing LoadBalancer services.

The secret has now the same format as the CSI driver. You need to rename:
* `key_id` to `access_key`,
* `access_key` to `secret_key`.

All other entries can be deleted.

If you use an EIM user, you also need to update your policies with [the updated EIM policy](../deploy/eim-policy.example.json).

# Usage

Some example of services:
- [2048](../examples/2048)
- [simple-lb](../examples/simple-lb)
- [simple-internal](../examples/simple-internal)
- [advanced-lb](../examples/advanced-lb)

Services can be annotated to fine-tune the configuration of the underlying Load Balancer Unit.
See [annotation documentation](../docs/annotations.md) for more details.

# Contributing

For feature requests or bug fixes, please [create an issue](https://github.com/outscale/cloud-provider-osc/issues).

If you want to help develop cloud-provider-osc, see the [development documentation](development.md).

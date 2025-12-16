# Outscale Cloud Controller Manager (CCM)

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/osc-cloud-controller-manager)](https://artifacthub.io/packages/search?repo=osc-cloud-controller-manager)
[![Project Graduated](https://docs.outscale.com/fr/userguide/_images/Project-Graduated-green.svg)](https://docs.outscale.com/en/userguide/Open-Source-Projects.html)


<p align="center">
  <img alt="Kubernetes Logo" src="https://upload.wikimedia.org/wikipedia/commons/3/39/Kubernetes_logo_without_workmark.svg" width="120px">
</p>

---

## 🌐 Links

* Project repo: [github.com/outscale/cloud-provider-osc](https://github.com/outscale/cloud-provider-osc)
* Helm chart: [osc-cloud-controller-manager](https://artifacthub.io/packages/helm/osc-cloud-controller-manager/osc-cloud-controller-manager)
* 📘 Documentation: [deployment documentation](../deploy/README.md)
* 🤝 Contribution Guide: [CONTRIBUTING.md](../CONTRIBUTING.md)
* 💬 Join us on [Discord](https://discord.gg/HUVtY5gT6s)

---

## 📄 Table of Contents

* [Overview](#-overview)
* [Installation](#-installation)
* [Usage](#-usage)
* [Contributing](#-contributing)
* [License](#-license)

---

## 🧭 Overview

The Outscale Cloud Controller Manager (cloud-provider-osc) provides the interface between a Kubernetes cluster and the OUTSCALE Cloud. 
This component is required to operate a cluster on the OUTSCALE Cloud.

More details on the [cloud-controller role](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) in a Kubernetes cluster.

### Features
- Node controller: manages nodes and node metadata,
- Service controller: allows creation of LoadBalancer services, based on [Load Balancer Units (LBU)](https://docs.outscale.com/en/userguide/About-Load-Balancers.html). 

---

## ⚙ Installation

### Kubernetes version support

Each Kubernetes version requires a specific CCM version.

CCM versions use the same major and minor version numbers as their corresponding Kubernetes releases. Patch version numbering is specific to the Outscale CCM and does not match Kubernetes patch releases.

E.g. Kubernetes v1.32.x will need CCM v1.32.y.

CCM v0.2.8 can be safely used with Kubernetes 1.30.x, and CCM v1.0.x can be safely used with Kubernetes 1.32.x.

CCM versions are available for Kubernetes 1.31, 1.32, and 1.33. As Kubernetes 1.31 has reached end of life (EOL), CCM v1.31 releases will be discontinued in the near future. Support for Kubernetes 1.34 will be added soon.

### Support matrix

| Kubernetes version | Recommended CCM version |
|--------------------|-------------------------|
| v1.30.x            | v0.2.8                  |
| v1.31.x            | v1.31.3                 |
| v1.32.x            | v1.32.3                 |
| v1.33.x            | v1.33.3                 |
| v1.34.x            | v1.34.3                 |

### Deployment on a new cluster

See the [deployment documentation](../deploy/README.md)

### Upgrading a cluster to a new Kubernetes version

When upgrading a cluster, the CCM needs to be upgraded for the target Kubernetes version before the creation of any kind of nodes (control-plane or worker).

Nodes created with a mismatched CCM version might not be properly configured.

### Upgrading CCM from v0 to v1

Annotations have changed, but the old ones still work. You do not need to update your existing LoadBalancer services.

The secret has now the same format as the CSI driver. You need to rename:
* `key_id` to `access_key`,
* `access_key` to `secret_key`.

All other entries can be deleted.

If you use an EIM user, you also need to update your policies with [the updated EIM policy](../deploy/eim-policy.example.json).

---

## 🚀 Usage

Some examples:
- [2048 game](../examples/2048)
- [Load-Balancer with access logs](../examples/access-logs)
- [Internal Load-Balancer](../examples/simple-internal)
- [Advanced configuration](../examples/advanced-lb)

Services can be annotated to fine-tune the configuration of the underlying Load Balancer Unit.
See [annotation documentation](../docs/annotations.md) for more details.

---

## 🤝 Contributing

For feature requests or bug fixes, please [create an issue](https://github.com/outscale/cloud-provider-osc/issues).

If you want to help develop cloud-provider-osc, see the [development documentation](development.md).

---

## 📜 License

**CAPOSC** is licensed under the BSD 3-Clause License.

© 2025 Outscale SAS

This project complies with the [REUSE Specification](https://reuse.software/).

See [LICENSES/](../LICENSES) directory for full license information.
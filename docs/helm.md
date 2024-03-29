# osc-cloud-controller-manager

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square) ![AppVersion: 0.2.3](https://img.shields.io/badge/AppVersion-0.2.3-informational?style=flat-square)

A Helm chart for OSC CCM cloud provider

**Homepage:** <https://github.com/outscale-dev/cloud-provider-osc/>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| 3DS Outscale | <support@outscale.com> |  |

## Source Code

* <https://github.com/outscale-dev/cloud-provider-osc/>

## Requirements

Kubernetes: `>=1.20.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| caBundle.key | string | `""` | Entry key in secret used to store additional certificates authorities |
| caBundle.name | string | `""` | Secret name containing additional certificates authorities |
| customEndpoint | string | `""` | Use customEndpoint (url with protocol) ex: https://api.eu-west-2.outscale.com/api/v1 |
| customEndpointEim | string | `""` | Use customEndpointEim (url with protocol) ex: https://eim.eu-west-2.outscale.com     |
| customEndpointFcu | string | `""` | Use customEndpointFcu (url with protocol) ex: https://fcu.eu-west-2.outscale.com |
| customEndpointLbu | string | `""` | Use customEndpointLbu (url with protocol) ex: https://lbu.eu-west-2.outscale.com   |
| httpsProxy | string | `""` | Value used to create environment variable HTTPS_PROXY |
| image.pullPolicy | string | `"IfNotPresent"` | Container pull policy |
| image.repository | string | `"outscale/cloud-provider-osc"` | Container image to use |
| image.tag | string | `"v0.2.3"` | Container image tag to deploy |
| imagePullSecrets | list | `[]` | Specify image pull secrets |
| noProxy | string | `""` | Value used to create environment variable NO_PROXY |
| nodeSelector | object | `{}` | Assign Pod to Nodes (see [kubernetes doc](https://kubernetes.io/docs/tasks/configure-pod-container/assign-pods-nodes/)) |
| oscSecretName | string | `"osc-secret"` | Secret name containing cloud credentials |
| podLabels | object | `{}` | Labels for pod |
| replicaCount | int | `1` | Number of replicas to deploy |
| tolerations | list | `[]` | Pod tolerations (see [kubernetes doc](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)) |
| verbose | int | `5` | Verbosity level of the plugin |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)

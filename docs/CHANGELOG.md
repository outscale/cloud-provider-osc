# Changelog

## [v1.33.0] - 2025-11-19

### ✨ Added
* ✨feat(loadbalancer): implement ipmode by @moh2a in https://github.com/outscale/cloud-provider-osc/pull/518
### 🛠️ Changed / Refactoring
* 🔊 logs: fix LBU response logging / switch to Go 1.25 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/519

## [v1.32.0] - 2025-11-19

### ✨ Added
* ✨feat(loadbalancer): implement ipmode by @moh2a in https://github.com/outscale/cloud-provider-osc/pull/518
### 🛠️ Changed / Refactoring
* 🔊 logs: fix LBU response logging / switch to Go 1.25 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/519

## [v1.31.0] - 2025-11-19

### ✨ Added
* ✨feat(loadbalancer): implement ipmode by @moh2a in https://github.com/outscale/cloud-provider-osc/pull/518
### 🛠️ Changed / Refactoring
* 🔊 logs: fix LBU response logging / switch to Go 1.25 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/519

## [helm-v0.7.0] - 2025-11-19

### 🐛 Fixed
* 🐛 fix(helm): fix empty volume/volumeMounts blocks by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/527

## [v1.0.1] - 2025-10-13

Change: by default, v1.0.1 only returns the hostname of a load-balancer ingress instead of hostname + IP.
If you need the IP, you will need to set the `service.beta.kubernetes.io/osc-load-balancer-ingress-address` annotation to `ip` or `both`.

### ✨ Added
* ✨ feat(loadbalancer): add service.beta.kubernetes.io/osc-load-balancer-ingress-address annotation by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/508
### 🐛 Fixed
* 🐛 fix: path was not configured on https health checks by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/510
* 🐛 fix: always create the LoadBalancer in the same net of its subnet by @alistarle and @jfbus in https://github.com/outscale/cloud-provider-osc/pull/509

## [v1.0.0] - 2025-10-01

No changes since v1.0.0-rc.1

Breaking change: the secret storing credentials has now the same format as the CSI driver

Changes since v0.2.8:
### ✨ Added
* ✨ feat(config): load cfg from profile file by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/462
* ✨ feat(loadbalancer): use predefined public IPs by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/463
* ✨ feat(loadbalancers): add custom tags  by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/464
* ✨ feat(loadbalancer): filter backend nodes by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/467
* ✨ feat: allow custom cluster id by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/470
* 👽️ load-balancer: set ingress IP for better integration with IP-based services by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/494
* 👽️ load-balancer: set ingress IP for private LBUs by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/498
### 🛠️ Changed / Refactoring
* 👷 dependabot: update to main branch by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/444
* ♻️ Version 1.0 refactoring by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/442
* 👷 build: update Go version by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/471
* 🔊 logs: use v1 logs for metadata calls by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/476
* 🚀 helm: add v0/v1 compatible helm chart by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/485
* ✅ tests(helm): fix tests by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/490
### 📝 Documentation
* 📝 examples: updated examples by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/489
* 📝 doc: updated README + sample EIM policy + cleanup by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/488
### 🐛 Fixed
* 🐛 fix/helm: nodeSelector did not work with RKE by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/436
* 🐛 fix(loadbalancer): CCM upgrade would recreate all listeners by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/469
* 🥅 errors(loadbalancers): better handling of nodes without providerID by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/481
* 🥅 errors: handle when no subnet is found by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/477
* 🐛 fix: ccm was broken outside eu-west-2 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/482
* 🐛 fix(loadbalancer): updating a proxy protocol LBU was broken by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/495
### 📦 Dependency updates
* Bump github.com/outscale/osc-sdk-go/v2 from 2.26.0 to 2.27.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/428
* ⬆ deps: bump kube to 1.32 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/437
* ⬆️ deps: Bump k8s.io/cloud-provider from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/446
* ⬆️ deps: Bump k8s.io/kubernetes from 1.32.3 to 1.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/451
* ⬆️ deps: Bump k8s.io/kubectl from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/449
* ⬆️ deps: Bump k8s.io/pod-security-admission from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/445
* ⬆️ deps: Bump github.com/stretchr/testify from 1.10.0 to 1.11.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/457
* ⬆️ deps: Bump github.com/outscale/osc-sdk-go/v2 from 2.27.0 to 2.29.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/454
* ⬆️ deps: Bump github.com/onsi/gomega from 1.36.3 to 1.38.2 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/458
* ⬆️ deps: Bump go.uber.org/mock from 0.5.2 to 0.6.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/459

## [v1.0.0-rc.1] - 2025-10-01

No changes since v1.0.0-beta.3

## [v1.0.0-beta.3] - 2025-09-24

### ✨ Added
* 👽️ load-balancer: set ingress IP for private LBUs by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/498

## [v1.0.0-beta.2] - 2025-09-19

### ✨ Added
* 👽️ load-balancer: set ingress IP for better integration with IP-based services by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/494
### 🐛 Fixed
* 🐛 fix(loadbalancer): updating a proxy protocol LBU was broken by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/495

## [v1.0.0-beta.1] - 2025-09-16

### 🛠️ Changed / Refactoring
* ✅ tests(helm): fix tests by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/490
### 📝 Documentation
* 📝 examples: updated examples by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/489
* 📝 doc: updated README + sample EIM policy + cleanup by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/488

## [v1.0.0-alpha.2] - 2025-09-11

### 🛠️ Changed / Refactoring
* 🚀 helm: add v0/v1 compatible helm chart by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/485
* 🔊 logs: use v1 logs for metadata calls by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/476
### 🐛 Fixed
* 🥅 errors(loadbalancers): better handling of nodes without providerID by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/481
* 🥅 errors: handle when no subnet is found by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/477
* 🐛 fix: ccm was broken outside eu-west-2 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/482


## [v1.0.0-alpha.1] - 2025-09-10

v1.0.0 is a major refactoring, fixing many bugs (no security group leftovers anymore), and adding new features & annotations.

A new set of annotations has been defined, but the v0.x annotations are still accepted for compatibility purposes.

### ✨ Added
* ✨ feat(config): load cfg from profile file by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/462
* ✨ feat(loadbalancer): use predefined public IPs by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/463
* ✨ feat(loadbalancers): add custom tags by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/464
* ✨ feat(loadbalancer): filter backend nodes by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/467
* ✨ feat: allow custom cluster id by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/470
### 🛠️ Changed / Refactoring
* ♻️ Version 1.0 refactoring by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/442
* 👷 build: update Go version by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/471
### 🐛 Fixed
* 🐛 fix/helm: nodeSelector did not work with RKE by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/436
* 🐛 fix(loadbalancer): CCM upgrade would recreate all listeners by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/469
### 📦 Dependency updates
* Bump github.com/outscale/osc-sdk-go/v2 from 2.26.0 to 2.27.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/428
* ⬆ deps: bump kube to 1.32 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/437
* ⬆️ deps: Bump k8s.io/cloud-provider from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/446
* ⬆️ deps: Bump k8s.io/kubernetes from 1.32.3 to 1.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/451
* ⬆️ deps: Bump k8s.io/kubectl from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/449
* ⬆️ deps: Bump k8s.io/pod-security-admission from 0.32.3 to 0.32.8 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/445
* ⬆️ deps: Bump github.com/stretchr/testify from 1.10.0 to 1.11.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/457
* ⬆️ deps: Bump github.com/outscale/osc-sdk-go/v2 from 2.27.0 to 2.29.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/454
* ⬆️ deps: Bump github.com/onsi/gomega from 1.36.3 to 1.38.2 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/458
* ⬆️ deps: Bump go.uber.org/mock from 0.5.2 to 0.6.0 by @dependabot[bot] in https://github.com/outscale/cloud-provider-osc/pull/459
### 🌱 Others
* 👷 dependabot: update to main branch by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/444
* 👷 github: updated templates by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/472

## [v0.2.8]
### ✨ Added
* 🔧 helm: add control-plane nodeSelector by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/415
### 🛠️ Changed
* 🚀 deploy: fix image version in osc-ccm-manifest.yml by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/392
* 👷 ci: add golangci-lint by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/396
* 👷 ci: update cred-scan workflow by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/411
* ⬆️ deps: bump Go SDK to v2.26.0, Kube to v1.30.12 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/416
* 📝 doc: deploy using the published Helm chart by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/418
* Bump k8s.io/kubectl from 0.30.12 to 0.30.13 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/423
* Bump k8s.io/kubernetes from 1.30.12 to 1.30.13 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/425
* Bump k8s.io/pod-security-admission from 0.30.12 to 0.30.13 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/424
* Bump k8s.io/cloud-provider from 0.30.12 to 0.30.13 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/426
* 👷 ci: fix trivy reporting by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/422
* 👷 ci: use cluster-api to build test cluster by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/417
* Bump k8s.io/cloud-provider from 0.30.13 to 0.30.14 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/431
* Bump k8s.io/kubectl from 0.30.13 to 0.30.14 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/430
* Bump github.com/prometheus/client_golang from 1.19.0 to 1.22.0 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/421
* Bump k8s.io/kubernetes from 1.30.13 to 1.30.14 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/429
* Bump k8s.io/pod-security-admission from 0.30.13 to 0.30.14 by @dependabot in https://github.com/outscale/cloud-provider-osc/pull/432
* 🚀 preparation for v0.2.8 by @jfbus in https://github.com/outscale/cloud-provider-osc/pull/433


## [v0.2.7]
### Changes
* Build: bump Go to 1.23.4 & distroless image ([#388](https://github.com/outscale/cloud-provider-osc/pull/388))
* Linter fixes, test cleanup ([#386](https://github.com/outscale/cloud-provider-osc/pull/386))
* Update import path ([#384](https://github.com/outscale/cloud-provider-osc/pull/384))
### Bugfixes
* Unable to set resources without caBundle ([#383](https://github.com/outscale/cloud-provider-osc/pull/383))
* Unhandled error in UpdateLoadBalancer ([#385](https://github.com/outscale/cloud-provider-osc/pull/385))

## [v0.2.6]
### Bugfixes
* add resource handlers in chart ([#381](https://github.com/outscale/cloud-provider-osc/pull/381))

## [v0.2.5]
### Bugfixes
* fix recommended k8s versions based on the version in the go.mod ([#379](https://github.com/outscale/cloud-provider-osc/pull/379))
* Clean way to set imagePullSecrets and respect list([#370]( https://github.com/outscale/cloud-provider-osc/pull/370))

## [v0.2.4]
### Bugfixes
* use buildx and wait more time lb to be up ([#327](https://github.com/outscale/cloud-provider-osc/pull/327))
* Make generic createSvc and createDeployment([#329]( https://github.com/outscale/cloud-provider-osc/pull/329))
* Create createOscClient ([#330](https://github.com/outscale/cloud-provider-osc/pull/330))
* Select VmType ([#339] (https://github.com/outscale/cloud-provider-osc/pull/339
Filters terminated Vms in getInstance ([#352] (https://github.com/outscale/cloud-provider-osc/pull/352))
* add debug tree to help troubelshooting ([#354] (https://github.com/outscale/cloud-provider-osc/pull/354))
* change deprecated master taint and role ([#355]( https://github.com/outscale/cloud-provider-osc/pull/355))
* update go v1.22.5 and k8s v0.30.2 ([#357] (https://github.com/outscale/cloud-provider-osc/pull/357))

## [v0.2.3]
### Bugfixes
* Can set customEndpoint for instancev2 ([#323](https://github.com/outscale/cloud-provider-osc/pull/323))

## [v0.2.2]
### Bugfixes
* Can set customEndpoint for api, fcu, lbu, eim ([#321](https://github.com/outscale/cloud-provider-osc/pull/321))

## [v0.2.1]
### Bugfixes
* Update osc-sdk-go package in order not to check region ([#319](https://github.com/outscale/cloud-provider-osc/pull/319))

## [v0.2.0]
### Features
* Support link  kubernetes node name and IaaS tag OscK8sNodeName ([#177](https://github.com/outscale/cloud-provider-osc/issues/177))
* Add NodeSelector and Tolerations in helm Chart ([#173](https://github.com/outscale/cloud-provider-osc/issues/173))
### Bugfixes
* Fix LoadBalancer creation in multi-AZ architecture ([#165](https://github.com/outscale/cloud-provider-osc/issues/165))
* Update IAM Policy ([#167](https://github.com/outscale/cloud-provider-osc/issues/167))
## [v0.1.1]
### Bugfixes
* Invalid zone in the metadata ([#149](https://github.com/outscale/cloud-provider-osc/issues/149)) 
## [v0.1.0]
### Notable changes
* Partial migration from AWS SDK to Outscale SDK ([#61](https://github.com/outscale/cloud-provider-osc/issues/61))
* Provide Region and Zone during node initialization ([#118](https://github.com/outscale/cloud-provider-osc/issues/118))
* Reduce log verbosity ([#64](https://github.com/outscale/cloud-provider-osc/issues/64))

### Bugfixes
* Implement workaround for the public cloud issue ([#68](https://github.com/outscale/cloud-provider-osc/issues/68)) 
    > **NOTE**: The actual solution is to not delete (in Public Cloud) the rule that allows all Public Cloud Loadbalancers to forward traffic to the cluster. 
## [v0.0.10beta]

### Notable changes
* Support the ability to label CCM pods ([#72](https://github.com/outscale/cloud-provider-osc/pull/72))
* Update to k8s v1.23.4 
### Bugfixes
* Handle deletion of old nodes ([#84](https://github.com/outscale/cloud-provider-osc/pull/84))

## [v0.0.9beta]

### Notable changes
* Update to k8s pkg 1.21.5
* update e2e tests

## [v0.0.8beta]

### Notable changes
* Make LB name configurable using annotations
## [v0.0.7beta]

### Notable changes
* Fix SG removals under vpc
## [v0.0.6beta]

### Notable changes
* Update k8s lib to 1.19.17 libs
* Support the InstanceV2 interface
* Add ginkgo e2e tests

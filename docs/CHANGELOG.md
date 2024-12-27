# Changelog

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

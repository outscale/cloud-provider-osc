# 🚀 Deploying the Outscale Cloud Controller Manager (CCM)

This documentation explains how to deploy Outscale Cloud Controller Manager (CCM).

## ✅ Requirements

You will need a Kubernetes cluster on the 3DS Outscale cloud.

### Controller Manager and Kubelet configuration

When running Kubernetes in the cloud, the `--cloud-provider external` flag is required on the following components:
* `kube-controller-manager`
* `kubelet`
* `kube-apiserver` (up to v1.33)

The flag has been removed from `kube-apiserver` in v1.33.

The configuration of this flag depends on the bootstrapping tool you use to deploy your cluster.
When using Cluster-API, the required configuration is:

```yaml
    clusterConfiguration:
      apiServer:
        extraArgs:
          cloud-provider: "external"
      controllerManager:
        extraArgs:
          cloud-provider: "external"
    [...]
    initConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
          cloud-provider: external
    [...]
    joinConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
          cloud-provider: external
```

More details can be found in the [Cloud Controller Manager Administration](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager) documentation.

### Nodes

Nodes should have a `spec.providerID` set with the following structure `osc:///<subregion>/<VM ID>`
(for compatibility purposes, `aws:///<subregion>/<VM ID>` is also supported).

### Networking

The CCM needs to access the [metadata server](https://docs.outscale.com/en/userguide/Accessing-the-Metadata-and-User-Data-of-an-Instance.html) in order to get information about nodes.

Access to `169.254.169.254/32` using TCP on port 80 (http) must be allowed from the control-plane nodes.

### Configuring Cloud Credentials

The CCM needs to access the Outscale API.

It is recommended to use a specific [Access Key](https://docs.outscale.com/en/userguide/About-Access-Keys.html) and create an [EIM user](https://docs.outscale.com/en/userguide/About-EIM-Users.html) with limited access. Check the [EIM policy example](eim-policy.example.json) to apply to such EIM user.

## ⚙️ Installation

> Each major Kubernetes release requires a specific version of the CCM. You will need to install the CCM version tailored for your Kubernetes version.

### Create Secret

Create a secret with your cloud credentials:
```shell
kubectl create secret generic osc-secret \
  --from-literal=access_key=$OSC_ACCESS_KEY --from-literal=secret_key=$OSC_SECRET_KEY \
  -n kube-system
```

### Deploy the Cloud Controller Manager

The CCM is deployed as a daemon set on control-plane nodes.

You can either deploy using a simple manifest:
```shell
kube_version=`kubectl get nodes --no-headers -o custom-columns=VERSION:.status.nodeInfo.kubeletVersion|cut -d . -f 1,2|head -1`
kubectl apply -f deploy/osc-ccm-manifest-$kube_version.yml
```

Or, you can use Helm:
```shell
helm upgrade --install --wait --wait-for-jobs k8s-osc-ccm oci://registry-1.docker.io/outscalehelm/osc-cloud-controller-manager --set oscSecretName=osc-secret --set image.tag=[the version compatible with your Kubernetes version]
```

More [helm options are available](../docs/helm.md)

## 🔖 Tagging

### Resource Tagging

The CCM expects resources to be tagged as being part of the cluster.

This includes:
- [Subnets](https://docs.outscale.com/en/userguide/About-Nets.html)
- [VMs](https://docs.outscale.com/en/userguide/About-VMs.html)
- [Security Groups](https://docs.outscale.com/en/userguide/About-Security-Groups.html)

The tag key must be `OscK8sClusterID/[cluster-id]` (`[cluster-id]` being the ID of a cluster) and tag value can be one of the following values:
- `shared`: resource is shared between multiple clusters, and should not be destroyed,
- `owned`: the resource is considered owned and managed by the cluster.

The CCM will fetch the `OscK8sClusterID` tag of the node it is running on and will expect to find the other resources with matching tag keys.

When using Cluster API Provider for Outscale (CAPOSC), the tag is automatically set, no additional steps are required.

### VM Tagging

The CCM is usually able to find VM instances using the `spec.providerID` value.

In some rare cases, the CCM will need a `OscK8sNodeName` tag on the VM, with the node name as a value.

When using Cluster API Provider for Outscale (CAPOSC), the tag is automatically set, no additional steps are required.

## 🚀 Creating load-balancers

### Subnets

The CCM will look for a subnet having one of the following tags:
* `OscK8sRole/service.internal` is service is internal,
* `OscK8sRole/service` is service is not internal or if no `OscK8sRole/service.internal` subnet is found,
* `OscK8sRole/loadbalancer` if no subnet found.

When using Cluster API Provider for Outscale (CAPOSC), the tags are automatically set, no additional steps are required.

### Security Groups

#### Ingress

By default, the service controller will automatically create a Security Group for each Load Balancer Unit (LBU) and will attach it to nodes in a VPC setup.

If you want to use a pre-created Security Group to be used, you can set the `service.beta.kubernetes.io/osc-load-balancer-security-group` annotation with the id of the security group to use.

You can also add additional security groups using the `service.beta.kubernetes.io/osc-load-balancer-extra-security-groups` annotation.

The CCM will automatically add manage ingress rules to allow traffic to the load-balancer.

You can set `service.Spec.LoadBalancerSourceRanges` to restrict trafic to a list of IP ranges.

#### Load-balancer to nodes

The CCM will add rules to allow trafic from the load-balancer to nodes.

Within node security groups, it will search for a security group having one of the following tags:
* `OscK8sRole/[role]`, with role being set with de `service.beta.kubernetes.io/osc-load-balancer-target-role` annotation (`worker` by default)
* `OscK8sMainSG/[cluster id]`.

The Cluster API Provider for Outscale (CAPOSC) sets a `OscK8sRole/worker` tag on all worker nodes, and allows you to add custom roles if needed.

## ⬆️ Upgrading CCM v0.x to v1.x

The secret has now the same format as the CSI driver. You need to rename:
* `key_id` to `access_key`,
* `access_key` to `secret_key`.

All other entries can be deleted.

If you use an EIM user, you also need to update your policies with [the updated EIM policy](eim-policy.example.json).
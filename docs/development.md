# Development

Resources regarding developping this ccm:
- [Cloud Controller Manager architecture](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)
- [Interfaces](https://github.com/kubernetes/cloud-provider/blob/master/cloud.go)
- [About legacy providers with CCM](https://github.com/kubernetes/legacy-cloud-providers)

# Pre-requisites

You will need a Kubernetes cluster to launch some tests and debug some behaviors.
You can use [osc-k8s-rke-cluster](https://github.com/outscale/osc-k8s-rke-cluster/) for this purpose.

You will also need a registry where to push your dev images. You can use whatever registry you want or install [a private registry](https://github.com/outscale/osc-k8s-rke-cluster/tree/master/addons/docker-registry) which is available with osc-k8s-rke-cluster.

# Building

`make` provide a quick reminder to all available commands:
```shell
$ make
help:
  - build              : build binary
  - build-image        : build Docker image
  - dockerlint         : check Dockerfile
  - verify             : check code
  - test               : run all tests
  - test-e2e-single-az : run e2e tests
```

Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `VERSION=dev make build-image`

# Push dev image to registry

If you are using the [private registry addon](https://github.com/outscale/osc-k8s-rke-cluster/tree/master/addons/docker-registry), start port-fowarding to access the registry:
```
./start_port_forwarding.sh
```

You can then push your dev image to your registry:
```
docker tag osc/cloud-provider-osc:dev localhost:4242/osc/cloud-provider-osc:dev
docker push localhost:4242/osc/cloud-provider-osc:dev
```

# Deploying

Make sure to copy, edit and deploy your own [secrets.yml](../deploy/secrets.example.yml):
```
kubectl apply -f deploy/secrets.yaml
```

**Replace only MY_AWS_ACCESS_KEY_ID with your outscale access key, MY_AWS_SECRET_ACCESS_KEY with your outscale secret key and MY_AWS_DEFAULT_REGION with your outscale region.**


Install/upgrade your CCM with your "dev" image:
```
helm upgrade --install --wait --wait-for-jobs k8s-osc-ccm deploy/k8s-osc-ccm --set image.pullPolicy="Always" --set oscSecretName=osc-secret --set image.repository=10.0.1.10:32500/osc/cloud-provider-osc --set image.tag=dev
```

Note: `10.0.1.10:32500` is provided by `start_port_forwarding.sh` script.

Check that CCM is deployed with:
```
kubectl get pod -n kube-system -l "app=osc-cloud-controller-manager"
```
If not re-created, you may want to rollout restart pods:
```
kubectl rollout restart daemonset osc-cloud-controller-manager -n kube-system
```

# Force node re-initialization

Once a node is initialized, node controller will not call cloud-controller-manager again to set its labels.
If you are working on a feature which require to update node labels, you may want to taint your node again:

```bash
kubectl taint nodes --all node.cloudprovider.kubernetes.io/uninitialized=true:NoSchedule
```

# Testing

* To execute all unit tests, run: `make test`
* To execute e2e single az tests, run: 
```bash
export OSC_ACCESS_KEY=YourSecretAccessKeyId
export OSC_SECRET_KEY=YourSecretAccessKey
export E2E_REGION="us-east-2" # default is "eu-west-2"
export E2E_AZ="us-east-2a" # default "eu-west-2a"
export KC=$(base64 -w 0 path/to/kube_config.yaml)
make test-e2e
```

# Quick build-push-deploy-test

Once your [secrets.yml](../deploy/secrets.example.yml) deployed and you registry available (e.g. `./start_port_forwarding.sh`),
you can speed up all the previous steps by running this all-in-one command:

```bash
OSC_ACCESS_KEY=YourSecretAccessKeyId \
OSC_SECRET_KEY=YourSecretAccessKey \
KC=$(base64 -w 0 path/to/kube_config.yaml) \
E2E_REGION="us-east-2" \
E2E_AZ="us-east-2a" \
VERSION=dev \
REGISTRY_IMAGE=localhost:4242/osc/cloud-provider-osc \
TARGET_IMAGE=10.0.1.10:32500/osc/cloud-provider-osc \
make build-image image-tag image-push helm_deploy test-e2e
```

# Release

## Helm release

1. In [CHANGELOG.md](CHANGELOG.md), add a new vX.Y.Z-helm version
2. Update chart version (if needed) in [Chart.yaml](../deploy/k8s-osc-ccm/Chart.yaml)
3. Update cloud-provider-osc version in [values.yaml](../deploy/k8s-osc-ccm/values.yaml) (listing all active container versions)
4. Generate helm doc `make helm-docs`
5. Update manifests `make helm-manifest`
6. Commit version with `git commit -am "cloud-controller-manager vX.Y.Z"`
7. Create PR and merge it to main
8. Tag & push the release
```shell
export HELM_VERSION=vX.Y.Z-helm
git tag -a $HELM_VERSION -m "🔖 Helm $HELM_VERSION"
git push origin $HELM_VERSION
```
11. Publish the release on Github

## Container release

There must be a release for each active Kubernetes release.
Each Kubernetes release has its own release branch (kubernetes-1.31, kubernetes-1.32, kubernetes-1.33, kubernetes-1.34)

Versioning is vX.Y.Z, where X.Y are the major/minor version numbers of each Kubernetes release.
Z is the version of the CCM, is increased at each release, and is the same for every Kubernetes release branch.

1. In [CHANGELOG.md](CHANGELOG.md), add a new version for every release branch
2. In [README.md](README.md), update the recommended version for each Kubernetes release
3. Create PR and merge it to main
4. Merge main into each release branch
```shell
git co kubernetes-X.Y
git merge main
git push origin kubernetes-X.Y
```
5. Check that CI is OK on every release branch
6. Tag release for each release branch
```shell
git co kubernetes-X.Y
git pull --rebase
export VERSION=vX.Y.Z
git tag -a $VERSION -m "🔖 CCM $VERSION"
git push origin $VERSION
```
7. Publish the releases on Github

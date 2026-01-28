# Node labels

This example shows how to label nodes with the following topology labels:
* `topology.outscale.com/cluster`
* `topology.outscale.com/server`

This allows deployments scenarios where:
* latency is critical, and pods should be deployed in the same cluster/on the same server,
* redundancy is critical, and pods should be deployed in different clusters/on different servers.

It is expected that nodes are deployed using [attract/repulse placement contraints](https://docs.outscale.com/en/userguide/Configuring-a-VM-with-User-Data-and-OUTSCALE-Tags.html#_adding_outscale_tags_in_user_data).

The example deploys `osc-labeler` as a daemonset.

- Deploy the example
```shell
kubectl apply -f examples/node-labels/example.yaml
```

- View cluster/server node labels
```shell
kubectl get nodes -o custom-columns=NAME:.metadata.name,CLUSTER:.metadata.labels.topology\\.outscale\\.com/cluster,SERVER:.metadata.labels.topology\\.outscale\\.com/server
```


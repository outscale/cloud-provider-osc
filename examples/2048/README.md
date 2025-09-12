# 2048 game
 
This example creates a deployment for the 2048 game exposed through a Service.

- Deploy the example
```sh
kubectl apply -f examples/2048/
```

- Ensure the LB is created and the endpoint is available
```sh
kubectl get svc -n 2048-game
NAME           TYPE           CLUSTER-IP    EXTERNAL-IP                                 PORT(S)        AGE
service-2048   LoadBalancer   10.40.78.20   2048-156698379.eu-west-2.lbu.outscale.com   80:31595/TCP   31s```
```

- Play the game using the URL specified in EXTERNAL-IP.

- Cleanup
```sh
kubectl delete -f examples/2048/
```
# Internal Load-Balancer
 
This example creates an echoheaders deployment on your cluster, which listens on port 8080 and is exposed through a service available only within the VPC.

- Verify the the subnet where the load balancer needs to be deployed has the `OskK8sRole/service.internal` tag (with any value).

- Deploy the example
```sh
$ kubectl apply -f examples/simple-internal/
namespace/simple-internal created
deployment.apps/echoheaders created
service/echoheaders-lb-internal created
```

- Ensure the LB is created and the endpoint is available
```sh
$ kubectl get svc -n simple-internal
NAME                      TYPE           CLUSTER-IP     EXTERNAL-IP                                                                      PORT(S)        AGE
echoheaders-lb-internal   LoadBalancer   10.40.92.204   internal-3485cbf3059b4a47b7febcda8d15e26b-126712148.eu-west-2.lbu.outscale.com   80:31595/TCP   10m
```

- Wait for the LB to be ready, then verify it is running and forwarding traffic
```sh
$ kubectl run --image curlimages/curl:8.14.1 example-curl --restart=Never -ti --rm -q -- curl -s -S http://internal-3485cbf3059b4a47b7febcda8d15e26b-126712148.eu-west-2.lbu.outscale.com

Hostname: echoheaders-5465f4df9d-wxht2

Pod Information:
	-no pod information available-

Server values:
	server_version=nginx: 1.13.3 - lua: 10008

Request Information:
	client_address=172.19.91.102
	method=GET
	real path=/
	query=
	request_version=1.1
	request_scheme=http
	request_uri=http://a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com:8080/

Request Headers:
	accept=*/*
	host=a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com
	user-agent=curl/7.29.0

Request Body:
	-no body in request-
```

- Cleanup resources:
```sh
$ kubectl delete  -f examples/simple-internal/
```




# Load-Balancer with access logs
 
This example creates a deployment named echoheaders on your cluster, listening on port 8080 and exposed to the internet.
Requests to the service are logged in an OOS bucket.

- Create bucket for logs 
```sh
$ aws s3 mb s3://my-bucket-name --endpoint https://oos.eu-west-2.outscale.com
```

> Replace `my-bucket-name` with the name of the bucket you want to use in `example.yaml` and the `aws` commands.

- Deploy the application
```sh
$ kubectl apply -f examples/access-logs/
namespace/access-logs created
deployment.apps/echoheaders created
service/echoheaders-lb-public created
```

- Ensure the LB is created and the endpoint is available
```sh
$ kubectl get svc -n access-logs
NAME                    TYPE           CLUSTER-IP     EXTERNAL-IP                                             PORT(S)        AGE
echoheaders-lb-public   LoadBalancer   10.40.36.210   access-logs-test-791236236.eu-west-2.lbu.outscale.com   80:31252/TCP   14m
```

- Check logs
```sh
aws s3 ls --recursive s3://my-bucket-name --endpoint https://oos.eu-west-2.outscale.com
```

- Wait for the LB to be ready, then verify it is running and forwarding traffic
```sh	
$ curl http://access-logs-test-791236236.eu-west-2.lbu.outscale.com

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

- Cleanup resources
```sh
$ kubectl delete  -f examples/access-logs/
```



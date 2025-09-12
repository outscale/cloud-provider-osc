# Advanced configuration
 
This example creates a deployment named echoheaders on your cluster. It listens on ports 8080 (HTTP) and 8443 (HTTPS), and is exposed through an HTTPS service.

> This example requires a SSL certificate. Read [the documentation about certificates](https://docs.outscale.com/en/userguide/Uploading-a-Server-Certificate.html) to learn how to upload a certificate.
    
- Update `example.yaml` by setting the certificate ssl ORN ID and the loadBalancerSourceRanges
```sh
$ OSC_ORN_ID="<the ID of your certificate>" ; \
  sed -i "s@OSC_ORN_ID@\"${OSC_ORN_ID}\"@g" ./examples/advanced-lb/example.yaml
$ MY_CIDR=`curl ifconfig.io`"/32" ; \
  sed -i "s@MY_CIDR@\"${MY_CIDR}\"@g" ./examples/advanced-lb/example.yaml
```

- Deploy the application
```sh
$ kubectl apply -f examples/advanced-lb/
namespace/advanced-lb created
deployment.apps/echoheaders created
service/echoheaders-lb-advanced-public created
```

- Ensure the LB is created and the endpoint is available
```sh	
$ kubectl get svc -n advanced-lb
NAME                             TYPE           CLUSTER-IP     EXTERNAL-IP                                                             PORT(S)                      AGE
echoheaders-lb-advanced-public   LoadBalancer   10.32.29.197   ad51051c7a133489591adc0e1fbec049-832076221.eu-west-2.lbu.outscale.com   80:31174/TCP,443:31249/TCP   84m
```


- Wait for the LB to be ready, then verify it is running and forwarding traffic
```sh
$ curl -k  https://ad51051c7a133489591adc0e1fbec049-832076221.eu-west-2.lbu.outscale.com/

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
$ kubectl delete  -f examples/advanced-lb/
```




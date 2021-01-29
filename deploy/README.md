# Deploy the OSC CCM
 
## Generate and apply the osc-secret 
```
	export OSC_ACCESS_KEY=XXXXX
    export OSC_SECRET_KEY=XXXXX
    
    
	
	cat ./deploy/secret_osc.yaml | \
	sed "s/secret_key: \"\"/secret_key: \"$OSC_SECRET_KEY\"/g" | \
    sed "s/access_key: \"\"/access_key: \"$OSC_ACCESS_KEY\"/g" > apply_secret.yaml
	
	cat apply_secret.yaml
	
	/usr/local/bin/kubectl delete -f apply_secret.yaml --namespace=kube-system
	/usr/local/bin/kubectl apply -f apply_secret.yaml --namespace=kube-system
```

## Deploy the CCM deamonset

```
	# set the IMAGE_SECRET, IMAGE_NAME, IMAGE_TAG, SECRET_NAME to the right values on your case
	IMAGE_SECRET=registry-dockerconfigjson && \
	IMAGE_NAME=registry.kube-system:5001/osc/cloud-provider-osc && \
	IMAGE_TAG=v1 && \
	SECRET_NAME=osc-secret 
	helm del --purge k8s-osc-ccm --tls
	helm install --name k8s-osc-ccm \
		--set imagePullSecrets=$IMAGE_SECRET \
		--set oscSecretName=$SECRET_NAME \
		--set image.repository=$IMAGE_NAME \
		--set image.tag=$IMAGE_TAG \
		deploy/k8s-osc-ccm --tls
		
	kubectl get pods -o wide -A -n kube-system | grep osc-cloud-controller-manager

```


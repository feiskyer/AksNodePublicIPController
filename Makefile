.PHONY: build push deploy

IMAGE_REGISTRY ?= feisky
IMAGE_NAME=aks-node-public-ip-controller
IMAGE_TAG=$(shell git describe --tags --dirty)
IMAGE=$(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

deploy:
	cat deploy.yaml | sed "s#docker.io/dgkanatsios/aksnodepublicipcontroller:0.2.8#${IMAGE}#g" | kubectl apply -f -

build:
	docker build -t ${IMAGE} .

push: build
	docker push ${IMAGE}


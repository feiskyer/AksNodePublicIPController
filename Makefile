.PHONY: build push deploy

IMAGE_REGISTRY ?= feisky
IMAGE_NAME=aks-node-public-ip-controller
IMAGE_TAG=$(shell git describe --tags --dirty)
IMAGE=$(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

deploy: push
	cat deploy.yaml | sed "s#docker.io/feisky/aks-node-public-ip-controller:0.2.8#${IMAGE}#g" | kubectl apply -f -

push: build
	docker push ${IMAGE}

build:
	docker build -t ${IMAGE} .

DOCKER_REPO=evanhutnik/wipercheck-service

build:
	docker build --tag ${DOCKER_REPO} .

push:
	docker push ${DOCKER_REPO}:latest

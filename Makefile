.PHONY: build docker-build deploy clean

build:
	go build -o bin/microservice .

docker-build:
	docker build -t k8s-microservice:latest .

deploy:
	kubectl apply -f k8s/deployment.yaml

clean:
	kubectl delete -f k8s/deployment.yaml
	rm -f bin/microservice
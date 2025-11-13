#!/bin/bash

eval $(minikube docker-env)
docker build -t raft-server:latest .
kubectl delete pod -l app=raft-node
kubectl apply -f k8s/deployment.yaml
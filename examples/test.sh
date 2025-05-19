#!/bin/bash

set -euxo pipefail

kubectl apply -f ./http-nginx.yaml
kubectl apply -f ./tcp-nginx.yaml
kubectl apply -f ./udp-example.yaml

openssl req -newkey rsa:4096 \
            -x509 \
            -sha256 \
            -days 3650 \
            -nodes \
            -out example.crt \
            -keyout example.key \
            -subj "/C=na/ST=na/L=na/O=na/OU=na/CN=na"

kubectl delete secret example-secret || true
kubectl create secret tls example-secret --cert=example.crt --key=example.key
kubectl apply -f ./https-nginx.yaml


#!/bin/bash

set -x

kubectl delete secret ccm-linode -n kube-system
kubectl delete serviceaccount ccm-linode -n kube-system
kubectl delete clusterrolebinding system:ccm-linode -n kube-system
kubectl delete daemonset ccm-linode -n kube-system

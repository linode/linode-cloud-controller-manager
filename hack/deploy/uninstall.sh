#!/bin/bash
set -x

kubectl delete deployment -l app=ccm-linode -n kube-system
kubectl delete service -l app=ccm-linode -n kube-system
kubectl delete serviceaccount -l app=ccm-linode -n kube-system
kubectl delete clusterrolebindings -l app=ccm-linode -n kube-system
kubectl delete clusterrole -l app=ccm-linode -n kube-system

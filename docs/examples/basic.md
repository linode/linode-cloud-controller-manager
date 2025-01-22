# Basic Services

## HTTP LoadBalancer

Basic HTTP LoadBalancer service with nginx:

```yaml
kind: Service
apiVersion: v1
metadata:
  name: http-lb
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
spec:
  type: LoadBalancer
  selector:
    app: nginx-http-example
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 80

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-http-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx-http-example
  template:
    metadata:
      labels:
        app: nginx-http-example
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80
          protocol: TCP
```

## HTTPS LoadBalancer

HTTPS LoadBalancer with TLS termination:

```yaml
kind: Service
apiVersion: v1
metadata:
  name: https-lb
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-throttle: "4"
    service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "tls-secret-name": "example-secret",
        "protocol": "https"
      }
spec:
  type: LoadBalancer
  selector:
    app: nginx-https-example
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
    - name: https
      protocol: TCP
      port: 443
      targetPort: https

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-https-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx-https-example
  template:
    metadata:
      labels:
        app: nginx-https-example
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
          - name: http
            containerPort: 80
            protocol: TCP
          - name: https
            containerPort: 443
            protocol: TCP
```

For more configuration options, see:
- [Service Annotations](../configuration/annotations.md)
- [LoadBalancer Configuration](../configuration/loadbalancer.md)

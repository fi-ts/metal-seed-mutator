apiVersion: apps/v1
kind: Deployment
metadata:
  name: metal-seed-mutator
  namespace: default
spec:
  selector:
    matchLabels:
      app: metal-seed-mutator
  template:
    metadata:
      labels:
        app: metal-seed-mutator
    spec:
      securityContext:
        runAsUser: 999
      containers:
        - name: metal-seed-mutator
          image: REGISTRY/fi-ts/metal-seed-mutator:latest
          # command:
          #  - /metal-seed-mutator
          #  - --mutations=gardenlet
          ports:
          - containerPort: 8080
            protocol: TCP
          volumeMounts:
            - name: tls
              mountPath: "/etc/metal-seed-mutator/"
      volumes:
        - name: tls
          secret:
            secretName: metal-seed-mutator-certs
---
apiVersion: v1
kind: Service
metadata:
  name: metal-seed-mutator
  namespace: default
spec:
  ports:
    - port: 443
      protocol: TCP
      targetPort: 8080
  selector:
    app: metal-seed-mutator

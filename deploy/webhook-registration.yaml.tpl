apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: metal-seed-mutator
webhooks:
- admissionReviewVersions:
  - v1beta1
  clientConfig:
    caBundle: CA_BUNDLE
    service:
      name: metal-seed-mutator
      namespace: default
      port: 443
  failurePolicy: Ignore
  matchPolicy: Exact
  name: metal-seed-mutator.metal-stack.dev
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - garden
  rules:
  - apiGroups:
    - apps
    apiVersions:
    - v1
    operations:
    - CREATE
    - UPDATE
    resources:
    - deployments
    scope: 'Namespaced'
  sideEffects: None

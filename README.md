# n8n Workflow Operator

A Kubernetes operator for declaratively managing n8n workflows using Custom Resource Definitions (CRDs).

## Motivation

Managing n8n workflows in a GitOps environment is challenging:

- The n8n CLI (`n8n import:workflow`) only updates the database
- Workflows must be manually toggled in the UI to reload webhooks
- No native Kubernetes-friendly way to manage workflows as code

This operator solves these problems by using the n8n REST API, which properly registers webhooks when activating workflows.

## Features

- Declarative workflow management via CRDs
- Automatic workflow sync to n8n instance
- Proper webhook registration via REST API
- GitOps-friendly (works with FluxCD, ArgoCD)
- Status reporting and health checks

## Quick Start

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy operator
kubectl apply -f config/manager/

# Create a workflow
kubectl apply -f examples/workflow.yaml
```

## Example

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: my-workflow
  namespace: n8n
spec:
  active: true
  workflow:
    name: "My Workflow"
    nodes:
      - id: "webhook"
        name: "Webhook"
        type: "n8n-nodes-base.webhook"
        parameters:
          httpMethod: "POST"
          path: "my-endpoint"
        position: [0, 0]
    connections: {}
    settings:
      executionOrder: "v1"
```

## Development

See [CLAUDE.md](./CLAUDE.md) for detailed implementation instructions.

```bash
# Run locally
make run

# Build
make build

# Test
make test
```

## License

Apache 2.0

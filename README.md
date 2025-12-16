# n8n Resource Operator

A Kubernetes operator for declaratively managing n8n resources (workflows, credentials) using Custom Resource Definitions (CRDs).

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
- Automatic cleanup on deletion (finalizers)
- Per-workflow n8n instance targeting

## Quick Start

### Prerequisites

- Kubernetes cluster (1.19+)
- n8n instance with API access enabled
- n8n API key

### Installation

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Create namespace
kubectl create namespace n8n-resource-operator-system

# Create secret with n8n API key
kubectl create secret generic n8n-api-key \
  --from-literal=api-key=YOUR_N8N_API_KEY \
  -n n8n-resource-operator-system

# Deploy operator
make deploy IMG=registry.registry.svc.cluster.local:5000/n8n-resource-operator:latest
```

### Create a Workflow

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: my-workflow
  namespace: n8n
spec:
  active: true
  n8nRef:
    name: n8n-service
    namespace: n8n
    secretRef:
      name: n8n-api-key
      key: api-key
  workflow:
    name: "My Workflow"
    nodes:
      - id: "webhook"
        name: "Webhook"
        type: "n8n-nodes-base.webhook"
        typeVersion: 1
        parameters:
          httpMethod: "POST"
          path: "my-endpoint"
        position: [0, 0]
    connections: {}
    settings:
      executionOrder: "v1"
```

Apply it:

```bash
kubectl apply -f my-workflow.yaml
```

Check status:

```bash
kubectl get n8nworkflows -n n8n
```

## Configuration

### N8nWorkflow Spec

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `active` | boolean | Whether the workflow should be active | `true` |
| `n8nRef.name` | string | n8n service name | `n8n-service` |
| `n8nRef.namespace` | string | n8n service namespace | Same as workflow |
| `n8nRef.port` | integer | n8n service port | `5678` |
| `n8nRef.secretRef` | object | Secret reference for API key | - |
| `workflow.name` | string | Workflow name (required) | - |
| `workflow.nodes` | array | Workflow nodes | - |
| `workflow.connections` | object | Node connections | - |
| `workflow.settings` | object | Workflow settings | - |

### Environment Variables

The operator supports these environment variables for default configuration:

| Variable | Description | Default |
|----------|-------------|---------|
| `N8N_API_URL` | Default n8n API URL | `http://n8n-service.n8n.svc.cluster.local:5678` |
| `N8N_API_KEY` | Default API key (use secrets in production) | - |

## Converting Existing Workflows

Use the included script to convert n8n JSON exports to CRs:

```bash
./scripts/convert-workflow.py workflow.json > workflow-cr.yaml
./scripts/convert-workflow.py --namespace n8n workflow.json > workflow-cr.yaml
```

## Development

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- kubebuilder

### Build

```bash
# Generate manifests and build
make build

# Run tests
make test

# Build Docker image
make docker-build IMG=registry.registry.svc.cluster.local:5000/n8n-resource-operator:dev

# Run locally (requires kubeconfig)
make run
```

### Deploy to Development Cluster

```bash
# Install CRDs
make install

# Deploy controller
make deploy IMG=registry.registry.svc.cluster.local:5000/n8n-resource-operator:dev

# View logs
kubectl logs -f deployment/n8n-resource-operator-controller-manager \
  -n n8n-resource-operator-system
```

### Cleanup

```bash
make undeploy
make uninstall
```

## Status Fields

The operator updates these status fields:

| Field | Description |
|-------|-------------|
| `workflowId` | The n8n internal workflow ID |
| `active` | Current activation state in n8n |
| `lastSyncTime` | Last successful sync timestamp |
| `webhookUrl` | Webhook URL if workflow has webhook trigger |
| `conditions` | Ready/Synced conditions |

## License

Apache 2.0

# n8n Resource Operator

A Kubernetes operator for declaratively managing [n8n](https://n8n.io/) workflows using Custom Resource Definitions (CRDs). Deploy and manage your n8n workflows as code with GitOps.

## Why This Operator?

Managing n8n workflows in a GitOps environment is challenging:

- The n8n CLI (`n8n import:workflow`) only updates the database, not the running process
- Workflows must be manually toggled in the UI to reload webhooks
- No native Kubernetes-friendly way to manage workflows as code

This operator solves these problems by using the **n8n REST API**, which properly registers webhooks when activating workflows.

## Features

- **Declarative workflow management** via Kubernetes CRDs
- **GitOps-friendly** - works seamlessly with FluxCD, ArgoCD, or any GitOps tool
- **Sync policies** - control how changes sync between Git and n8n UI
- **Proper webhook registration** via REST API (not CLI)
- **Multi-instance support** - manage multiple n8n instances (cloud and self-hosted)
- **Centralized secrets** - API keys stored in operator namespace
- **Status reporting** - track workflow state, webhook URLs, and sync status
- **Automatic cleanup** - workflows are deleted from n8n when CRs are removed

## Quick Start

### Prerequisites

- Kubernetes cluster (1.19+)
- n8n instance with [API access enabled](https://docs.n8n.io/api/)
- n8n API key

### Installation

```bash
# Install CRDs
kubectl apply -f https://raw.githubusercontent.com/jspanos/n8n-resource-operator/main/config/crd/bases/n8n.slys.dev_n8ninstances.yaml
kubectl apply -f https://raw.githubusercontent.com/jspanos/n8n-resource-operator/main/config/crd/bases/n8n.slys.dev_n8nworkflows.yaml

# Create namespace
kubectl create namespace n8n-resource-operator

# Deploy operator (replace with your image)
kubectl apply -f https://raw.githubusercontent.com/jspanos/n8n-resource-operator/main/config/deploy/
```

### Step 1: Create an N8nInstance

First, define your n8n connection in the operator namespace:

```yaml
# For self-hosted n8n in Kubernetes
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: default
  namespace: n8n-resource-operator  # Must be in operator namespace
spec:
  serviceRef:
    name: n8n-service
    namespace: n8n
    port: 5678
  credentials:
    secretName: n8n-api-key
    secretKey: api-key
```

```yaml
# For n8n.cloud (cloud-hosted n8n)
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: cloud
  namespace: n8n-resource-operator
spec:
  url: "https://myorg.app.n8n.cloud"
  credentials:
    secretName: n8n-cloud-api-key
    secretKey: api-key
```

Create the API key secret (in the operator namespace):

```bash
kubectl create secret generic n8n-api-key \
  --namespace n8n-resource-operator \
  --from-literal=api-key=YOUR_N8N_API_KEY
```

### Step 2: Create Your First Workflow

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: hello-webhook
  namespace: n8n  # Workflows can be in any namespace
spec:
  # Reference to the N8nInstance (by name, in operator namespace)
  instanceRef: default

  # Sync policy: Always, CreateOnly, or Manual
  syncPolicy: Always

  # Whether the workflow should be active
  active: true

  # The workflow definition
  workflow:
    name: "Hello Webhook"
    nodes:
      - id: "webhook"
        name: "Webhook"
        type: "n8n-nodes-base.webhook"
        typeVersion: 2
        parameters:
          httpMethod: "POST"
          path: "hello"
          responseMode: "lastNode"
        position: [0, 0]
      - id: "respond"
        name: "Respond"
        type: "n8n-nodes-base.respondToWebhook"
        typeVersion: 1
        parameters:
          respondWith: "json"
          responseBody: '={"message": "Hello from n8n!", "timestamp": "{{ $now }}"}'
        position: [200, 0]
    connections:
      Webhook:
        main:
          - - node: "Respond"
              type: "main"
              index: 0
    settings:
      executionOrder: "v1"
```

Apply it:

```bash
kubectl apply -f hello-webhook.yaml
```

Check status:

```bash
# Check N8nInstance health
kubectl get n8ninstances -n n8n-resource-operator

# Output:
# NAME      URL                                           READY   LAST CHECK             AGE
# default   http://n8n-service.n8n.svc.cluster.local:5678 true    2024-01-15T10:30:00Z   5m

# Check workflows
kubectl get n8nworkflows -n n8n

# Output:
# NAME            INSTANCE   WORKFLOW NAME    ACTIVE   SYNC POLICY   WORKFLOW ID   AGE
# hello-webhook   default    Hello Webhook    true     Always        abc123xyz     5m
```

## Configuration

### N8nInstance Spec

The N8nInstance CRD defines a connection to an n8n instance. All N8nInstance resources must be created in the operator namespace.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `url` | string | Full n8n API URL (for cloud/external n8n) | - |
| `serviceRef.name` | string | n8n Kubernetes service name | - |
| `serviceRef.namespace` | string | n8n service namespace | - |
| `serviceRef.port` | integer | n8n service port | `5678` |
| `credentials.secretName` | string | Secret containing API key (required) | - |
| `credentials.secretKey` | string | Key in secret for API key | `api-key` |

> **Note:** Either `url` OR `serviceRef` must be specified, but not both.

### N8nWorkflow Spec

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `instanceRef` | string | Name of N8nInstance in operator namespace (required) | - |
| `syncPolicy` | string | How to sync with n8n (see below) | `Always` |
| `active` | boolean | Whether workflow should be active | `true` |
| `workflow.name` | string | Workflow name in n8n (required) | - |
| `workflow.nodes` | array | Workflow nodes | - |
| `workflow.connections` | object | Node connections | - |
| `workflow.settings` | object | Workflow settings | - |

### Sync Policies

Control how the operator handles synchronization between your CRD and the n8n UI:

| Policy | Description | Use Case |
|--------|-------------|----------|
| `Always` | Continuously sync, overwriting UI changes | Production workflows, strict GitOps |
| `CreateOnly` | Create workflow once, never update | Development - allows UI editing |
| `Manual` | Pause all sync operations | Active development in UI |

**Example: Development Workflow**

```yaml
spec:
  instanceRef: default
  syncPolicy: CreateOnly  # Create once, then allow UI editing
  active: true
  workflow:
    name: "My Development Workflow"
    # ...
```

### Status Fields

**N8nInstance Status:**

| Field | Description |
|-------|-------------|
| `ready` | Whether the instance is reachable and authenticated |
| `url` | Resolved URL for the n8n instance |
| `lastHealthCheck` | Last successful health check timestamp |
| `conditions` | Ready condition |

**N8nWorkflow Status:**

| Field | Description |
|-------|-------------|
| `workflowId` | The n8n internal workflow ID |
| `active` | Current activation state in n8n |
| `lastSyncTime` | Last successful sync timestamp |
| `webhookUrl` | Webhook URL if workflow has webhook trigger |
| `conditions` | Ready/Synced conditions |

## Multi-Instance Support

This operator supports multiple n8n instances, allowing you to:

- Manage workflows across different n8n environments (dev, staging, prod)
- Target both self-hosted Kubernetes n8n and cloud-hosted n8n.cloud
- Keep secrets centralized in the operator namespace

**Example: Multiple Instances**

```yaml
# Self-hosted development n8n
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: dev
  namespace: n8n-resource-operator
spec:
  serviceRef:
    name: n8n-service
    namespace: n8n-dev
  credentials:
    secretName: n8n-dev-api-key
---
# Production n8n.cloud
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: production
  namespace: n8n-resource-operator
spec:
  url: "https://mycompany.app.n8n.cloud"
  credentials:
    secretName: n8n-production-api-key
---
# Workflow targeting production
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: production-workflow
  namespace: n8n
spec:
  instanceRef: production  # Target the production instance
  active: true
  workflow:
    name: "Production Workflow"
    # ...
```

## Converting Existing Workflows

Export your workflow from n8n (Settings > Download) and use this approach:

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: my-workflow
spec:
  instanceRef: default
  active: true
  workflow:
    # Paste the contents of your exported JSON here
    name: "My Workflow"
    nodes: [...]
    connections: {...}
    settings: {...}
```

## GitOps Integration

### FluxCD Example

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: n8n
resources:
  - workflows/my-workflow.yaml
  - workflows/another-workflow.yaml
```

### ArgoCD Example

Point ArgoCD at a directory containing your N8nWorkflow manifests.

## Migration from v0.2.x

Version 0.3.0 introduces breaking changes. Follow these steps to migrate:

### Breaking Changes

1. `spec.n8nRef` has been **removed** from N8nWorkflow
2. `n8n.apiUrl` and `n8n.apiKey.*` Helm values have been **removed**
3. Secrets must now be in the **operator namespace**
4. N8nInstance CRD is now **required** before deploying workflows

### Migration Steps

1. **Create N8nInstance resources** for your n8n instances:

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: default
  namespace: n8n-resource-operator
spec:
  serviceRef:
    name: n8n-service
    namespace: n8n
    port: 5678
  credentials:
    secretName: n8n-api-key
```

2. **Move API key secrets** to the operator namespace:

```bash
kubectl get secret n8n-api-key -n n8n -o yaml | \
  sed 's/namespace: n8n/namespace: n8n-resource-operator/' | \
  kubectl apply -f -
```

3. **Update N8nWorkflow resources** - replace `n8nRef` with `instanceRef`:

```yaml
# Before (v0.2.x)
spec:
  n8nRef:
    name: n8n-service
    namespace: n8n
    secretRef:
      name: n8n-api-key

# After (v0.3.0)
spec:
  instanceRef: default  # Reference to N8nInstance name
```

4. **Update Helm values** if using Helm - remove the old `n8n.*` configuration:

```yaml
# Before (v0.2.x)
n8n:
  apiUrl: "http://n8n-service.n8n.svc.cluster.local:5678"
  apiKey:
    existingSecret: "n8n-api-key"
    secretKey: "api-key"

# After (v0.3.0)
# (no n8n section - configure via N8nInstance CRD)
```

5. **Deploy v0.3.0** operator

## Development

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- kubebuilder 3.x

### Build

```bash
# Generate manifests
make manifests

# Build binary
make build

# Run tests
make test

# Build Docker image
make docker-build IMG=your-registry/n8n-resource-operator:dev

# Push image
make docker-push IMG=your-registry/n8n-resource-operator:dev
```

### Run Locally

```bash
# Install CRDs
make install

# Run controller (uses your kubeconfig)
# Note: You must set POD_NAMESPACE or use --operator-namespace flag
POD_NAMESPACE=n8n-resource-operator make run
```

### Deploy to Cluster

```bash
make deploy IMG=your-registry/n8n-resource-operator:latest
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

Apache 2.0 - see [LICENSE](LICENSE) for details.

## Acknowledgments

- [n8n](https://n8n.io/) - The workflow automation platform
- [Kubebuilder](https://kubebuilder.io/) - SDK for building Kubernetes operators

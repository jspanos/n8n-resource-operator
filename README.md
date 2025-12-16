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
- **Multi-instance support** - target different n8n instances per workflow
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
kubectl apply -f https://raw.githubusercontent.com/jspanos/n8n-resource-operator/main/config/crd/bases/n8n.slys.dev_n8nworkflows.yaml

# Create namespace
kubectl create namespace n8n-resource-operator-system

# Deploy operator (replace with your image)
kubectl apply -f https://raw.githubusercontent.com/jspanos/n8n-resource-operator/main/config/deploy/
```

### Create Your First Workflow

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: hello-webhook
  namespace: n8n
spec:
  # n8n instance configuration
  n8nRef:
    url: "http://n8n.example.com:5678"  # Or use service discovery (see below)
    secretRef:
      name: n8n-api-key
      key: api-key

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
kubectl get n8nworkflows -n n8n

# Output:
# NAME            WORKFLOW NAME    ACTIVE   SYNC POLICY   WORKFLOW ID        LAST SYNC              AGE
# hello-webhook   Hello Webhook    true     Always        abc123xyz          2024-01-15T10:30:00Z   5m
```

## Configuration

### N8nWorkflow Spec

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `n8nRef.url` | string | Full n8n API URL (takes precedence) | - |
| `n8nRef.name` | string | n8n Kubernetes service name | `n8n-service` |
| `n8nRef.namespace` | string | n8n service namespace | Same as workflow |
| `n8nRef.port` | integer | n8n service port | `5678` |
| `n8nRef.secretRef` | object | Secret containing API key | - |
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
  syncPolicy: CreateOnly  # Create once, then allow UI editing
  active: true
  workflow:
    name: "My Development Workflow"
    # ...
```

### n8n Instance Configuration

**Option 1: Direct URL (Recommended for external n8n)**

```yaml
spec:
  n8nRef:
    url: "https://n8n.mycompany.com"
    secretRef:
      name: n8n-api-key
      key: api-key
```

**Option 2: Kubernetes Service Discovery**

```yaml
spec:
  n8nRef:
    name: n8n-service      # Service name
    namespace: n8n         # Service namespace
    port: 5678             # Service port
    secretRef:
      name: n8n-api-key
      key: api-key
```

**Option 3: Environment Variables (Operator Default)**

Set these on the operator deployment:

```yaml
env:
  - name: N8N_API_URL
    value: "http://n8n-service.n8n.svc.cluster.local:5678"
  - name: N8N_API_KEY
    valueFrom:
      secretKeyRef:
        name: n8n-api-key
        key: api-key
```

### Status Fields

The operator reports these status fields:

| Field | Description |
|-------|-------------|
| `workflowId` | The n8n internal workflow ID |
| `active` | Current activation state in n8n |
| `lastSyncTime` | Last successful sync timestamp |
| `webhookUrl` | Webhook URL if workflow has webhook trigger |
| `conditions` | Ready/Synced conditions |

## Converting Existing Workflows

Export your workflow from n8n (Settings > Download) and use this approach:

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: my-workflow
spec:
  n8nRef:
    url: "http://n8n.example.com:5678"
    secretRef:
      name: n8n-api-key
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
make run
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

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) for building this n8n Workflow Operator.

## Project Overview

Build a Kubernetes operator that manages n8n workflows declaratively using Custom Resource Definitions (CRDs). The operator watches for N8nWorkflow custom resources and syncs them to a running n8n instance using the n8n REST API.

## Problem Statement

Currently, deploying n8n workflows to Kubernetes requires:
1. Manually importing workflows via n8n CLI (`n8n import:workflow`)
2. CLI only updates the database, not the running process
3. Workflows must be manually toggled in UI to reload
4. No GitOps-friendly way to manage workflows as code

## Solution

Create an operator that:
1. Defines N8nWorkflow CRD for declarative workflow management
2. Uses n8n REST API to create/update/activate/deactivate workflows
3. Integrates with FluxCD GitOps workflow
4. Properly handles webhook registration (the main pain point)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                              │
│  ┌──────────────────┐     ┌──────────────────────────────┐ │
│  │  FluxCD          │     │  n8n-resource-operator       │ │
│  │  ┌────────────┐  │     │  ┌─────────────────────────┐ │ │
│  │  │ Git Repo   │──┼────▶│  │ Controller              │ │ │
│  │  │ (workflows)│  │     │  │ - Watch N8nWorkflow CRs │ │ │
│  │  └────────────┘  │     │  │ - Reconcile via API     │ │ │
│  └──────────────────┘     │  └───────────┬─────────────┘ │ │
│                           └──────────────┼───────────────┘ │
│                                          │ REST API        │
│                                          ▼                 │
│                           ┌──────────────────────────────┐ │
│                           │  n8n Instance                │ │
│                           │  - POST /api/v1/workflows    │ │
│                           │  - POST /api/v1/workflows/   │ │
│                           │        {id}/activate         │ │
│                           └──────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## n8n REST API Reference

Base URL: `http://n8n-service.n8n.svc.cluster.local:5678`
Authentication: `X-N8N-API-KEY` header

### Workflow Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/workflows` | List all workflows |
| GET | `/api/v1/workflows/{id}` | Get workflow by ID |
| POST | `/api/v1/workflows` | Create new workflow |
| PUT | `/api/v1/workflows/{id}` | Update existing workflow |
| DELETE | `/api/v1/workflows/{id}` | Delete workflow |
| POST | `/api/v1/workflows/{id}/activate` | Activate workflow |
| POST | `/api/v1/workflows/{id}/deactivate` | Deactivate workflow |

### Key Insight - Activation via API

When using the REST API (not CLI), activation properly registers webhooks in the running process. This is the key difference from CLI-based activation which only updates the database.

### Credential Endpoints (Future)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/credentials` | List credentials |
| POST | `/api/v1/credentials` | Create credential |
| DELETE | `/api/v1/credentials/{id}` | Delete credential |

### Example API Calls

```bash
# List workflows
curl -H "X-N8N-API-KEY: $API_KEY" http://localhost:5678/api/v1/workflows

# Create workflow
curl -X POST -H "X-N8N-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d @workflow.json \
  http://localhost:5678/api/v1/workflows

# Activate workflow
curl -X POST -H "X-N8N-API-KEY: $API_KEY" \
  http://localhost:5678/api/v1/workflows/{id}/activate
```

## CRD Design

### N8nWorkflow CRD

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nWorkflow
metadata:
  name: generate-workout
  namespace: n8n
spec:
  # Reference to n8n instance (for multi-tenant setups)
  n8nRef:
    name: n8n-service
    namespace: n8n

  # Workflow should be active after deployment
  active: true

  # The workflow definition (n8n JSON format)
  workflow:
    name: "Generate Workout"
    nodes:
      - id: "webhook-1"
        name: "Webhook"
        type: "n8n-nodes-base.webhook"
        parameters:
          httpMethod: "POST"
          path: "generate"
          responseMode: "responseNode"
        position: [0, 0]
      # ... more nodes
    connections:
      Webhook:
        main:
          - - node: "Next Node"
              type: "main"
              index: 0
    settings:
      executionOrder: "v1"

status:
  # Operator-managed status fields
  workflowId: "fLdJnQejH26Iljqw"
  active: true
  lastSyncTime: "2025-01-15T10:30:00Z"
  webhookUrl: "/webhook/generate"
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-01-15T10:30:00Z"
      reason: "WorkflowSynced"
      message: "Workflow successfully synced and activated"
```

### N8nInstance CRD (Optional - for multi-tenant)

```yaml
apiVersion: n8n.slys.dev/v1alpha1
kind: N8nInstance
metadata:
  name: n8n-main
  namespace: n8n
spec:
  # Service reference
  serviceRef:
    name: n8n-service
    port: 5678

  # API key secret reference
  apiKeySecretRef:
    name: n8n-api-key
    key: api-key

status:
  ready: true
  version: "1.70.0"
  lastHealthCheck: "2025-01-15T10:30:00Z"
```

## Implementation Requirements

### Language & Framework

- **Go** with **controller-runtime** (Kubebuilder or Operator SDK)
- Alternatively: **Rust** with **kube-rs** for a lighter footprint

### Core Controller Logic

```go
func (r *N8nWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the N8nWorkflow CR
    workflow := &n8nv1alpha1.N8nWorkflow{}
    if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Get n8n API client
    n8nClient, err := r.getN8nClient(ctx, workflow.Spec.N8nRef)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 3. Check if workflow exists in n8n
    existing, err := n8nClient.GetWorkflowByName(workflow.Spec.Workflow.Name)

    if existing == nil {
        // 4a. Create new workflow
        created, err := n8nClient.CreateWorkflow(workflow.Spec.Workflow)
        if err != nil {
            return ctrl.Result{}, err
        }
        workflow.Status.WorkflowId = created.ID
    } else {
        // 4b. Update existing workflow
        workflow.Status.WorkflowId = existing.ID
        if needsUpdate(existing, workflow.Spec.Workflow) {
            _, err := n8nClient.UpdateWorkflow(existing.ID, workflow.Spec.Workflow)
            if err != nil {
                return ctrl.Result{}, err
            }
        }
    }

    // 5. Handle activation state
    if workflow.Spec.Active && !workflow.Status.Active {
        if err := n8nClient.ActivateWorkflow(workflow.Status.WorkflowId); err != nil {
            return ctrl.Result{}, err
        }
        workflow.Status.Active = true
    } else if !workflow.Spec.Active && workflow.Status.Active {
        if err := n8nClient.DeactivateWorkflow(workflow.Status.WorkflowId); err != nil {
            return ctrl.Result{}, err
        }
        workflow.Status.Active = false
    }

    // 6. Update status
    workflow.Status.LastSyncTime = metav1.Now()
    if err := r.Status().Update(ctx, workflow); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

### n8n API Client

```go
type N8nClient struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func (c *N8nClient) CreateWorkflow(workflow WorkflowSpec) (*WorkflowResponse, error) {
    body, _ := json.Marshal(workflow)
    req, _ := http.NewRequest("POST", c.baseURL+"/api/v1/workflows", bytes.NewReader(body))
    req.Header.Set("X-N8N-API-KEY", c.apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    // ... handle response
}

func (c *N8nClient) ActivateWorkflow(id string) error {
    req, _ := http.NewRequest("POST", c.baseURL+"/api/v1/workflows/"+id+"/activate", nil)
    req.Header.Set("X-N8N-API-KEY", c.apiKey)

    resp, err := c.httpClient.Do(req)
    // ... handle response
}
```

## FluxCD Integration

### Directory Structure in Galaxy Repo

```
kubernetes/
├── apps/
│   └── n8n-resource-operator/
│       ├── kustomization.yaml
│       └── deployment.yaml
├── infrastructure/
│   └── n8n/
│       ├── workflows/
│       │   ├── kustomization.yaml
│       │   ├── generate-workout.yaml      # N8nWorkflow CR
│       │   ├── exercise-gif-search.yaml   # N8nWorkflow CR
│       │   └── youtube-to-gif.yaml        # N8nWorkflow CR
│       └── api-key-sealed.yaml            # Sealed secret for API key
└── crds/
    └── n8n-resource-operator/
        └── n8n.slys.dev_n8nworkflows.yaml
```

### Example FluxCD Kustomization

```yaml
# kubernetes/infrastructure/n8n/workflows/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: n8n

resources:
  - generate-workout.yaml
  - exercise-gif-search.yaml
  - youtube-to-gif.yaml
```

## Development Phases

### Phase 1: MVP (Workflows Only)
- [ ] CRD definition for N8nWorkflow
- [ ] Basic controller with create/update/delete
- [ ] Activation/deactivation support
- [ ] Status reporting
- [ ] Health checks

### Phase 2: Robustness
- [ ] Retry logic with exponential backoff
- [ ] Conflict resolution (workflow name collisions)
- [ ] Webhook URL validation
- [ ] Events for debugging
- [ ] Metrics (Prometheus)

### Phase 3: Credentials (Future)
- [ ] N8nCredential CRD
- [ ] Secret reference support
- [ ] Credential sync

### Phase 4: Advanced (Future)
- [ ] N8nInstance CRD for multi-tenant
- [ ] Workflow execution triggers
- [ ] Backup/restore support

## Testing Strategy

### Unit Tests
- Controller reconciliation logic
- n8n API client mocking
- CRD validation

### Integration Tests
- Deploy to kind cluster
- Test against real n8n instance
- Verify webhook registration works

### E2E Tests
- Full GitOps flow with FluxCD
- Workflow changes via Git commits

## Build & Deploy

### Makefile Targets

```makefile
# Generate CRD manifests
make manifests

# Build controller binary
make build

# Build Docker image
make docker-build IMG=ghcr.io/jspanos/n8n-resource-operator:latest

# Deploy to cluster
make deploy IMG=ghcr.io/jspanos/n8n-resource-operator:latest

# Run locally (for development)
make run
```

### Helm Chart (Optional)

Consider creating a Helm chart for easier distribution.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `N8N_API_URL` | n8n API base URL | `http://n8n-service.n8n:5678` |
| `N8N_API_KEY` | API key (or use secret ref) | - |
| `RECONCILE_INTERVAL` | How often to reconcile | `5m` |
| `LOG_LEVEL` | Logging verbosity | `info` |

### RBAC Requirements

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: n8n-resource-operator
rules:
  - apiGroups: ["n8n.slys.dev"]
    resources: ["n8nworkflows", "n8nworkflows/status"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
```

## Existing Infrastructure Context

The operator will be deployed to a Kubernetes cluster managed by FluxCD:

- **GitOps Repo**: `galaxy` repository with FluxCD
- **n8n Namespace**: `n8n`
- **n8n Service**: `n8n-service.n8n.svc.cluster.local:5678`
- **Storage**: Local-path provisioner for PVCs
- **Secrets**: Bitnami Sealed Secrets

## References

- [n8n REST API Documentation](https://docs.n8n.io/api/)
- [n8n API Reference](https://docs.n8n.io/api/api-reference/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Operator SDK](https://sdk.operatorframework.io/)
- [kube-rs (Rust)](https://kube.rs/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)

## Getting Started

```bash
# Initialize operator project
kubebuilder init --domain slys.dev --repo github.com/jspanos/n8n-resource-operator

# Create API
kubebuilder create api --group n8n --version v1alpha1 --kind N8nWorkflow

# Implement controller logic
# Edit: api/v1alpha1/n8nworkflow_types.go
# Edit: controllers/n8nworkflow_controller.go

# Generate manifests
make manifests

# Run tests
make test

# Build and push
make docker-build docker-push IMG=ghcr.io/jspanos/n8n-resource-operator:v0.1.0
```

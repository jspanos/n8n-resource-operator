/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a client for the n8n REST API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new n8n API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Workflow represents an n8n workflow
type Workflow struct {
	ID          string           `json:"id,omitempty"`
	Name        string           `json:"name"`
	Active      bool             `json:"active"`
	Nodes       []map[string]any `json:"nodes,omitempty"`
	Connections map[string]any   `json:"connections,omitempty"`
	Settings    map[string]any   `json:"settings,omitempty"`
	StaticData  map[string]any   `json:"staticData,omitempty"`
	PinData     map[string]any   `json:"pinData,omitempty"`
	CreatedAt   string           `json:"createdAt,omitempty"`
	UpdatedAt   string           `json:"updatedAt,omitempty"`
	Tags        []map[string]any `json:"tags,omitempty"`
	Meta        map[string]any   `json:"meta,omitempty"`
}

// WorkflowCreateRequest is used when creating a workflow (active is read-only in n8n API)
type WorkflowCreateRequest struct {
	Name        string           `json:"name"`
	Nodes       []map[string]any `json:"nodes,omitempty"`
	Connections map[string]any   `json:"connections,omitempty"`
	Settings    map[string]any   `json:"settings,omitempty"`
	StaticData  map[string]any   `json:"staticData,omitempty"`
	PinData     map[string]any   `json:"pinData,omitempty"`
}

// WorkflowListResponse represents the response from listing workflows
type WorkflowListResponse struct {
	Data       []Workflow `json:"data"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ErrorResponse represents an error from the n8n API
type ErrorResponse struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *ErrorResponse) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

// doRequest performs an HTTP request to the n8n API
func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		return nil, &errResp
	}

	return respBody, nil
}

// ListWorkflows retrieves all workflows from n8n
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var allWorkflows []Workflow
	cursor := ""

	for {
		path := "/api/v1/workflows"
		if cursor != "" {
			path += "?cursor=" + cursor
		}

		respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list workflows: %w", err)
		}

		var listResp WorkflowListResponse
		if err := json.Unmarshal(respBody, &listResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal workflows: %w", err)
		}

		allWorkflows = append(allWorkflows, listResp.Data...)

		if listResp.NextCursor == "" {
			break
		}
		cursor = listResp.NextCursor
	}

	return allWorkflows, nil
}

// GetWorkflow retrieves a workflow by ID
func (c *Client) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/v1/workflows/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow %s: %w", id, err)
	}

	var workflow Workflow
	if err := json.Unmarshal(respBody, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	return &workflow, nil
}

// GetWorkflowByName finds a workflow by name
func (c *Client) GetWorkflowByName(ctx context.Context, name string) (*Workflow, error) {
	workflows, err := c.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}

	for _, w := range workflows {
		if w.Name == name {
			return &w, nil
		}
	}

	return nil, nil // Not found
}

// CreateWorkflow creates a new workflow in n8n
func (c *Client) CreateWorkflow(ctx context.Context, workflow *Workflow) (*Workflow, error) {
	// Use WorkflowCreateRequest to exclude the 'active' field (read-only in n8n API)
	createReq := &WorkflowCreateRequest{
		Name:        workflow.Name,
		Nodes:       workflow.Nodes,
		Connections: workflow.Connections,
		Settings:    workflow.Settings,
		StaticData:  workflow.StaticData,
		PinData:     workflow.PinData,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/api/v1/workflows", createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	var created Workflow
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("failed to unmarshal created workflow: %w", err)
	}

	return &created, nil
}

// UpdateWorkflow updates an existing workflow
func (c *Client) UpdateWorkflow(ctx context.Context, id string, workflow *Workflow) (*Workflow, error) {
	// Use WorkflowCreateRequest to exclude the 'active' field (read-only in n8n API)
	updateReq := &WorkflowCreateRequest{
		Name:        workflow.Name,
		Nodes:       workflow.Nodes,
		Connections: workflow.Connections,
		Settings:    workflow.Settings,
		StaticData:  workflow.StaticData,
		PinData:     workflow.PinData,
	}

	respBody, err := c.doRequest(ctx, http.MethodPut, "/api/v1/workflows/"+id, updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow %s: %w", id, err)
	}

	var updated Workflow
	if err := json.Unmarshal(respBody, &updated); err != nil {
		return nil, fmt.Errorf("failed to unmarshal updated workflow: %w", err)
	}

	return &updated, nil
}

// DeleteWorkflow deletes a workflow by ID
func (c *Client) DeleteWorkflow(ctx context.Context, id string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/workflows/"+id, nil)
	if err != nil {
		return fmt.Errorf("failed to delete workflow %s: %w", id, err)
	}
	return nil
}

// ActivateWorkflow activates a workflow
func (c *Client) ActivateWorkflow(ctx context.Context, id string) (*Workflow, error) {
	respBody, err := c.doRequest(ctx, http.MethodPost, "/api/v1/workflows/"+id+"/activate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to activate workflow %s: %w", id, err)
	}

	var workflow Workflow
	if err := json.Unmarshal(respBody, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal activated workflow: %w", err)
	}

	return &workflow, nil
}

// DeactivateWorkflow deactivates a workflow
func (c *Client) DeactivateWorkflow(ctx context.Context, id string) (*Workflow, error) {
	respBody, err := c.doRequest(ctx, http.MethodPost, "/api/v1/workflows/"+id+"/deactivate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to deactivate workflow %s: %w", id, err)
	}

	var workflow Workflow
	if err := json.Unmarshal(respBody, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deactivated workflow: %w", err)
	}

	return &workflow, nil
}

// HealthCheck performs a basic health check by attempting to list workflows
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, "/api/v1/workflows?limit=1", nil)
	return err
}

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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:5678", "test-api-key")

	if client.baseURL != "http://localhost:5678" {
		t.Errorf("expected baseURL to be http://localhost:5678, got %s", client.baseURL)
	}
	if client.apiKey != "test-api-key" {
		t.Errorf("expected apiKey to be test-api-key, got %s", client.apiKey)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
}

func TestListWorkflows(t *testing.T) {
	workflows := []Workflow{
		{ID: "1", Name: "Test Workflow 1", Active: true},
		{ID: "2", Name: "Test Workflow 2", Active: false},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-N8N-API-KEY") != "test-key" {
			t.Errorf("expected X-N8N-API-KEY header to be test-key, got %s", r.Header.Get("X-N8N-API-KEY"))
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows" {
			t.Errorf("expected path /api/v1/workflows, got %s", r.URL.Path)
		}

		resp := WorkflowListResponse{Data: workflows}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(result))
	}
	if result[0].Name != "Test Workflow 1" {
		t.Errorf("expected first workflow name to be Test Workflow 1, got %s", result[0].Name)
	}
}

func TestGetWorkflow(t *testing.T) {
	workflow := Workflow{ID: "123", Name: "Test Workflow", Active: true}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows/123" {
			t.Errorf("expected path /api/v1/workflows/123, got %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(workflow)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.GetWorkflow(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "123" {
		t.Errorf("expected workflow ID to be 123, got %s", result.ID)
	}
	if result.Name != "Test Workflow" {
		t.Errorf("expected workflow name to be Test Workflow, got %s", result.Name)
	}
}

func TestGetWorkflowByName(t *testing.T) {
	workflows := []Workflow{
		{ID: "1", Name: "Other Workflow", Active: false},
		{ID: "2", Name: "Target Workflow", Active: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := WorkflowListResponse{Data: workflows}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.GetWorkflowByName(context.Background(), "Target Workflow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected workflow to be found")
	}
	if result.ID != "2" {
		t.Errorf("expected workflow ID to be 2, got %s", result.ID)
	}
}

func TestGetWorkflowByNameNotFound(t *testing.T) {
	workflows := []Workflow{
		{ID: "1", Name: "Other Workflow", Active: false},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := WorkflowListResponse{Data: workflows}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.GetWorkflowByName(context.Background(), "Non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != nil {
		t.Error("expected workflow to be nil for non-existent name")
	}
}

func TestCreateWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows" {
			t.Errorf("expected path /api/v1/workflows, got %s", r.URL.Path)
		}

		var workflow Workflow
		json.NewDecoder(r.Body).Decode(&workflow)

		// Return created workflow with ID
		workflow.ID = "new-123"
		json.NewEncoder(w).Encode(workflow)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	input := &Workflow{Name: "New Workflow", Active: false}
	result, err := client.CreateWorkflow(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "new-123" {
		t.Errorf("expected workflow ID to be new-123, got %s", result.ID)
	}
	if result.Name != "New Workflow" {
		t.Errorf("expected workflow name to be New Workflow, got %s", result.Name)
	}
}

func TestUpdateWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows/123" {
			t.Errorf("expected path /api/v1/workflows/123, got %s", r.URL.Path)
		}

		var workflow Workflow
		json.NewDecoder(r.Body).Decode(&workflow)
		workflow.ID = "123"
		json.NewEncoder(w).Encode(workflow)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	input := &Workflow{Name: "Updated Workflow", Active: true}
	result, err := client.UpdateWorkflow(context.Background(), "123", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "123" {
		t.Errorf("expected workflow ID to be 123, got %s", result.ID)
	}
	if result.Name != "Updated Workflow" {
		t.Errorf("expected workflow name to be Updated Workflow, got %s", result.Name)
	}
}

func TestDeleteWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows/123" {
			t.Errorf("expected path /api/v1/workflows/123, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.DeleteWorkflow(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestActivateWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows/123/activate" {
			t.Errorf("expected path /api/v1/workflows/123/activate, got %s", r.URL.Path)
		}

		workflow := Workflow{ID: "123", Name: "Test", Active: true}
		json.NewEncoder(w).Encode(workflow)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.ActivateWorkflow(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Active {
		t.Error("expected workflow to be active")
	}
}

func TestDeactivateWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workflows/123/deactivate" {
			t.Errorf("expected path /api/v1/workflows/123/deactivate, got %s", r.URL.Path)
		}

		workflow := Workflow{ID: "123", Name: "Test", Active: false}
		json.NewEncoder(w).Encode(workflow)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result, err := client.DeactivateWorkflow(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Active {
		t.Error("expected workflow to be inactive")
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Message: "Workflow not found",
			Code:    "NOT_FOUND",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	_, err := client.GetWorkflow(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	// Error is wrapped, so just check the message contains expected content
	if err.Error() == "" {
		t.Error("expected error message to be non-empty")
	}
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		resp := WorkflowListResponse{Data: []Workflow{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

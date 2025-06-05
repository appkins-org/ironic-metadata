package metadata

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/appkins-org/ironic-metadata/pkg/client"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
)

// createTestHandler creates a handler for testing.
func createTestHandler() *Handler {
	return &Handler{
		Clients: &client.Clients{},
	}
}

func TestHandler_Routes(t *testing.T) {
	handler := createTestHandler()
	router := handler.Routes()

	if router == nil {
		t.Fatal("Routes() returned nil")
	}
}

func TestHandler_handleOpenStackRoot(t *testing.T) {
	handler := createTestHandler()

	req, err := http.NewRequest("GET", "/openstack", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	handler.handleOpenStackRoot(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var versions []string
	err = json.Unmarshal(rr.Body.Bytes(), &versions)
	if err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if len(versions) == 0 {
		t.Error("expected at least one version in response")
	}

	if versions[0] != "latest" {
		t.Errorf("expected first version to be 'latest', got %s", versions[0])
	}
}

func TestHandler_handleLatestRoot(t *testing.T) {
	handler := createTestHandler()

	req, err := http.NewRequest("GET", "/openstack/latest", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	handler.handleLatestRoot(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var endpoints []string
	err = json.Unmarshal(rr.Body.Bytes(), &endpoints)
	if err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	expectedEndpoints := []string{
		"meta_data.json",
		"network_data.json",
		"user_data",
		"vendor_data.json",
		"vendor_data2.json",
	}

	if len(endpoints) != len(expectedEndpoints) {
		t.Errorf("expected %d endpoints, got %d", len(expectedEndpoints), len(endpoints))
	}
}

func TestBuildMetaData(t *testing.T) {
	handler := createTestHandler()

	now := time.Now()
	node := &nodes.Node{
		UUID:      "test-uuid-123",
		Name:      "test-node",
		CreatedAt: now,
		Owner:     "test-project",
		InstanceInfo: map[string]any{
			"public_keys": map[string]any{
				"default": "ssh-rsa AAAAB3NzaC1yc2EAAAADA...",
			},
		},
		Properties: map[string]any{
			"memory_mb": "8192",
			"cpus":      "4",
		},
	}

	metaData := handler.buildMetaData(node)

	if metaData.UUID != node.UUID {
		t.Errorf("expected UUID %s, got %s", node.UUID, metaData.UUID)
	}

	if metaData.Name != node.Name {
		t.Errorf("expected Name %s, got %s", node.Name, metaData.Name)
	}

	if metaData.ProjectID != node.Owner {
		t.Errorf("expected ProjectID %s, got %s", node.Owner, metaData.ProjectID)
	}

	if len(metaData.PublicKeys) == 0 {
		t.Error("expected public keys to be populated")
	}

	if len(metaData.Meta) == 0 {
		t.Error("expected meta properties to be populated")
	}
}

func TestBuildNetworkData(t *testing.T) {
	handler := createTestHandler()

	node := &nodes.Node{
		UUID: "test-uuid-123",
		Name: "test-node",
	}

	networkData := handler.buildNetworkData(node)

	if networkData == nil {
		t.Fatal("buildNetworkData returned nil")
	}

	// Check that basic network structure is created.
	if len(networkData.Links) == 0 {
		t.Error("expected at least one network link")
	}

	if len(networkData.Networks) == 0 {
		t.Error("expected at least one network")
	}
}

func TestGetNodeHostname(t *testing.T) {
	tests := []struct {
		name     string
		node     *nodes.Node
		expected string
	}{
		{
			name: "with name",
			node: &nodes.Node{
				UUID: "test-uuid",
				Name: "test-node",
			},
			expected: "test-node",
		},
		{
			name: "without name",
			node: &nodes.Node{
				UUID: "test-uuid",
				Name: "",
			},
			expected: "test-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNodeHostname(tt.node)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name: "X-Forwarded-For header",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1, 10.0.0.1",
			},
			remoteAddr: "127.0.0.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name: "X-Real-IP header",
			headers: map[string]string{
				"X-Real-IP": "192.168.1.2",
			},
			remoteAddr: "127.0.0.1:12345",
			expected:   "192.168.1.2",
		},
		{
			name:       "Remote address only",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.3:12345",
			expected:   "192.168.1.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := getClientIP(req)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

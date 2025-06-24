package metadata

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/appkins-org/ironic-metadata/pkg/client"
)

func TestParseDHCPLeaseFile(t *testing.T) {
	tests := []struct {
		name          string
		leaseContent  string
		targetIP      string
		expectedMAC   string
		expectedError bool
	}{
		{
			name: "valid lease file with target IP",
			leaseContent: `1750802648 9c:6b:00:70:59:8b 10.1.105.195 * *
1750802648 9c:6b:00:70:59:8a 10.1.105.194 * *`,
			targetIP:    "10.1.105.195",
			expectedMAC: "9c:6b:00:70:59:8b",
		},
		{
			name: "valid lease file with different target IP",
			leaseContent: `1750802648 9c:6b:00:70:59:8b 10.1.105.195 * *
1750802648 9c:6b:00:70:59:8a 10.1.105.194 * *`,
			targetIP:    "10.1.105.194",
			expectedMAC: "9c:6b:00:70:59:8a",
		},
		{
			name: "IP not found in lease file",
			leaseContent: `1750802648 9c:6b:00:70:59:8b 10.1.105.195 * *
1750802648 9c:6b:00:70:59:8a 10.1.105.194 * *`,
			targetIP:      "10.1.105.196",
			expectedError: true,
		},
		{
			name:          "empty lease file",
			leaseContent:  "",
			targetIP:      "10.1.105.195",
			expectedError: true,
		},
		{
			name: "malformed lease entries",
			leaseContent: `invalid line
1750802648 9c:6b:00:70:59:8b 10.1.105.195 * *
another invalid line`,
			targetIP:    "10.1.105.195",
			expectedMAC: "9c:6b:00:70:59:8b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary lease file
			tmpFile, err := os.CreateTemp("", "dhcp_lease_*.txt")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write test content
			if _, err := tmpFile.WriteString(tt.leaseContent); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			tmpFile.Close()

			// Test the parsing function
			mac, err := parseDHCPLeaseFile(tmpFile.Name(), tt.targetIP)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if mac != tt.expectedMAC {
				t.Errorf("Expected MAC %s, got %s", tt.expectedMAC, mac)
			}
		})
	}
}

func TestLookupNodeByMAC(t *testing.T) {
	// Create a mock handler (this would need a proper mock client for full testing)
	handler := &Handler{
		Clients: &client.Clients{},
	}

	// Create temporary lease file
	tmpFile, err := os.CreateTemp("", "dhcp_lease_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	leaseContent := `1750802648 9c:6b:00:70:59:8b 10.1.105.195 * *
1750802648 9c:6b:00:70:59:8a 10.1.105.194 * *`

	if _, err := tmpFile.WriteString(leaseContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Note: This test would require mocking the Ironic client to fully test
	// For now, we just test that the DHCP parsing part works
	mac, err := parseDHCPLeaseFile(tmpFile.Name(), "10.1.105.195")
	if err != nil {
		t.Errorf("Unexpected error parsing DHCP lease: %v", err)
		return
	}

	expectedMAC := "9c:6b:00:70:59:8b"
	if mac != expectedMAC {
		t.Errorf("Expected MAC %s, got %s", expectedMAC, mac)
	}

	// Test with context timeout (would need proper mocking for full test)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// This would fail because we don't have a real Ironic client
	_, err = handler.lookupNodeByMAC(ctx, "10.1.105.195")
	if err == nil {
		t.Error("Expected error due to missing Ironic client, but got none")
	}
}

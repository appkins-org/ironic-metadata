#!/bin/bash

# Example script to create a test node in Ironic with metadata
# This demonstrates how to configure a node with the required instance_info
# for the metadata service to return meaningful data

set -e

# Configuration
IRONIC_URL="${IRONIC_URL:-http://localhost:6385}"
NODE_NAME="${NODE_NAME:-test-node-01}"
NODE_UUID="${NODE_UUID:-$(uuidgen)}"
CLIENT_IP="${CLIENT_IP:-192.168.1.100}"

echo "Creating test node in Ironic"
echo "Node Name: $NODE_NAME"
echo "Node UUID: $NODE_UUID"
echo "Client IP: $CLIENT_IP"
echo "Ironic URL: $IRONIC_URL"
echo "================================"

# User data (cloud-init configuration)
USER_DATA=$(cat <<EOF
#cloud-config
hostname: ${NODE_NAME}
users:
  - name: ubuntu
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... # Replace with your SSH key
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
package_update: true
packages:
  - curl
  - wget
  - htop
runcmd:
  - echo "Node ${NODE_NAME} initialized" > /var/log/node-init.log
EOF
)

# Network data (basic configuration)
NETWORK_DATA=$(cat <<'EOF'
{
  "links": [
    {
      "id": "eth0",
      "type": "physical",
      "ethernet_mac_address": "52:54:00:12:34:56",
      "mtu": 1500
    }
  ],
  "networks": [
    {
      "id": "network0",
      "type": "ipv4",
      "link": "eth0",
      "ip_address": "192.168.1.100",
      "netmask": "255.255.255.0",
      "gateway": "192.168.1.1",
      "dns": ["8.8.8.8", "8.8.4.4"]
    }
  ],
  "services": [
    {
      "type": "dns",
      "address": "8.8.8.8"
    },
    {
      "type": "dns", 
      "address": "8.8.4.4"
    }
  ]
}
EOF
)

# Public keys
PUBLIC_KEYS=$(cat <<'EOF'
{
  "default": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... test@example.com"
}
EOF
)

# Create the node using Ironic API
echo "Creating node..."

# First, create the basic node
curl -X POST "$IRONIC_URL/v1/nodes" \
  -H "Content-Type: application/json" \
  -H "X-OpenStack-Ironic-API-Version: 1.50" \
  -d "{
    \"uuid\": \"$NODE_UUID\",
    \"name\": \"$NODE_NAME\",
    \"driver\": \"fake-hardware\",
    \"properties\": {
      \"memory_mb\": 8192,
      \"cpus\": 4,
      \"local_gb\": 100,
      \"cpu_arch\": \"x86_64\"
    },
    \"driver_info\": {
      \"deploy_ramdisk_address\": \"$CLIENT_IP\"
    }
  }"

echo "Node created successfully!"

# Update instance_info with metadata
echo "Setting instance_info..."

curl -X PATCH "$IRONIC_URL/v1/nodes/$NODE_UUID" \
  -H "Content-Type: application/json" \
  -H "X-OpenStack-Ironic-API-Version: 1.50" \
  -d "{
    \"instance_info\": {
      \"user_data\": $(echo "$USER_DATA" | jq -Rs .),
      \"network_data\": $NETWORK_DATA,
      \"public_keys\": $PUBLIC_KEYS
    }
  }"

echo "Instance info updated successfully!"

echo
echo "Test node created! You can now test the metadata service:"
echo "  curl -H \"X-Forwarded-For: $CLIENT_IP\" http://localhost:8080/openstack/latest/meta_data.json"
echo
echo "To clean up, delete the node:"
echo "  curl -X DELETE $IRONIC_URL/v1/nodes/$NODE_UUID"

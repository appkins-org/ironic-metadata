#!/bin/bash

# Helper script to configure docker-compose.yml with the correct network interface
# This script will automatically detect the primary network interface and update docker-compose.yml

set -e

# Function to get the primary network interface
get_primary_interface() {
    # Try different methods to get the primary interface
    
    # Method 1: Check default route
    local interface=$(ip route | grep '^default' | awk '{print $5}' | head -n1)
    if [[ -n "$interface" ]]; then
        echo "$interface"
        return 0
    fi
    
    # Method 2: Check for common interface names
    for iface in ens3 ens33 enp0s3 enp0s8 eth0 wlan0; do
        if ip link show "$iface" >/dev/null 2>&1; then
            echo "$iface"
            return 0
        fi
    done
    
    # Method 3: Get first non-loopback interface
    local interface=$(ip link show | grep -E '^[0-9]+:' | grep -v 'lo:' | head -n1 | awk -F': ' '{print $2}')
    if [[ -n "$interface" ]]; then
        echo "$interface"
        return 0
    fi
    
    echo "eth0"  # fallback
}

echo "Detecting network configuration..."

# Get the primary interface
PRIMARY_IFACE=$(get_primary_interface)
echo "Detected primary network interface: $PRIMARY_IFACE"

# Check if docker-compose.yml exists
if [[ ! -f "docker-compose.yml" ]]; then
    echo "Error: docker-compose.yml not found in current directory"
    exit 1
fi

# Update docker-compose.yml with the correct interface
echo "Updating docker-compose.yml with interface: $PRIMARY_IFACE"
sed -i.bak "s/parent: eth0.*/parent: $PRIMARY_IFACE  # Auto-detected interface/" docker-compose.yml

echo "Configuration updated!"
echo "Original file backed up as docker-compose.yml.bak"
echo ""
echo "You can now run:"
echo "  docker-compose up -d ironic-metadata"
echo ""
echo "The metadata service will be available at: http://169.254.169.254/"
echo ""
echo "To verify the network interface is correct, check:"
echo "  ip link show $PRIMARY_IFACE"

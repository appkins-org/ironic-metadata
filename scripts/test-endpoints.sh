#!/bin/bash

# Test script for ironic-metadata service
# This script tests the various endpoints provided by the metadata service

set -e

# Configuration
METADATA_URL="${METADATA_URL:-http://localhost:8080}"
CLIENT_IP="${CLIENT_IP:-192.168.1.100}"

echo "Testing Ironic Metadata Service at $METADATA_URL"
echo "Simulating client IP: $CLIENT_IP"
echo "================================"

# Function to make curl requests with proper headers
make_request() {
    local endpoint="$1"
    local expected_content_type="$2"
    
    echo "Testing endpoint: $endpoint"
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}\nCONTENT_TYPE:%{content_type}" \
        -H "X-Forwarded-For: $CLIENT_IP" \
        "$METADATA_URL$endpoint")
    
    body=$(echo "$response" | sed -E '$d;$d')
    status_code=$(echo "$response" | grep "HTTPSTATUS:" | cut -d: -f2)
    content_type=$(echo "$response" | grep "CONTENT_TYPE:" | cut -d: -f2)
    
    echo "  Status: $status_code"
    echo "  Content-Type: $content_type"
    
    if [ "$status_code" = "200" ]; then
        echo "  ✅ Success"
        if [ -n "$expected_content_type" ] && [[ "$content_type" == *"$expected_content_type"* ]]; then
            echo "  ✅ Content-Type matches expected: $expected_content_type"
        fi
        echo "  Response body:"
        echo "$body" | sed 's/^/    /'
    elif [ "$status_code" = "404" ]; then
        echo "  ⚠️  Not Found (expected if no matching node)"
    else
        echo "  ❌ Failed with status $status_code"
        echo "  Response body:"
        echo "$body" | sed 's/^/    /'
    fi
    echo
}

# Test OpenStack endpoints
echo "OpenStack Metadata API Endpoints:"
echo "--------------------------------"

make_request "/openstack" "application/json"
make_request "/openstack/" "application/json"
make_request "/openstack/latest" "application/json"
make_request "/openstack/latest/" "application/json"
make_request "/openstack/latest/meta_data.json" "application/json"
make_request "/openstack/latest/network_data.json" "application/json"
make_request "/openstack/latest/user_data" "text/plain"
make_request "/openstack/latest/vendor_data.json" "application/json"
make_request "/openstack/latest/vendor_data2.json" "application/json"

# Test EC2-compatible endpoints
echo "EC2-Compatible Metadata API Endpoints:"
echo "-------------------------------------"

make_request "/" "text/plain"
make_request "/latest" "text/plain"
make_request "/latest/" "text/plain"
make_request "/latest/meta-data" "text/plain"
make_request "/latest/meta-data/" "text/plain"
make_request "/latest/user-data" "text/plain"

echo "Test completed!"
echo
echo "Note: 404 responses are expected if no Ironic node matches the client IP ($CLIENT_IP)"
echo "To test with a real node, ensure:"
echo "1. Ironic is running and accessible"
echo "2. A node exists with matching IP configuration"
echo "3. The node has instance_info with user_data, public_keys, etc."

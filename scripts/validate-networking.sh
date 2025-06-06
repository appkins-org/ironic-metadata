#!/bin/bash
#
# Validation script for ironic-metadata advanced networking
# Tests various networking configurations and connectivity
#

set -euo pipefail

METADATA_IP="169.254.169.254"
METADATA_PORT="80"
TIMEOUT=10

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

# Test basic connectivity to metadata service
test_basic_connectivity() {
    log_info "Testing basic connectivity to metadata service..."
    
    if curl -f -s --connect-timeout "$TIMEOUT" --max-time "$TIMEOUT" \
        "http://$METADATA_IP/openstack/latest/meta_data.json" > /dev/null 2>&1; then
        log_success "Metadata service is responding"
    else
        log_error "Metadata service is not responding"
    fi
}

# Test if metadata IP is configured
test_ip_configuration() {
    log_info "Testing IP configuration..."
    
    if ip addr show | grep -q "$METADATA_IP"; then
        log_success "Metadata IP $METADATA_IP is configured locally"
    else
        log_warning "Metadata IP $METADATA_IP not configured locally (may be routed)"
    fi
}

# Test routing to metadata IP
test_routing() {
    log_info "Testing routing to metadata IP..."
    
    if ip route get "$METADATA_IP" > /dev/null 2>&1; then
        local route_info
        route_info=$(ip route get "$METADATA_IP" 2>/dev/null | head -n1)
        log_success "Route to $METADATA_IP: $route_info"
    else
        log_error "No route to metadata IP $METADATA_IP"
    fi
}

# Test if service is listening
test_service_listening() {
    log_info "Testing if service is listening..."
    
    if netstat -ln 2>/dev/null | grep -q ":$METADATA_PORT.*$METADATA_IP" || \
       ss -ln 2>/dev/null | grep -q ":$METADATA_PORT.*$METADATA_IP"; then
        log_success "Service is listening on $METADATA_IP:$METADATA_PORT"
    else
        log_warning "Service may not be listening on $METADATA_IP:$METADATA_PORT"
    fi
}

# Test DNS resolution (should fail for link-local)
test_dns_resolution() {
    log_info "Testing DNS resolution..."
    
    if nslookup "$METADATA_IP" > /dev/null 2>&1; then
        log_warning "DNS resolution for $METADATA_IP found (unusual for link-local)"
    else
        log_success "DNS resolution correctly fails for link-local address"
    fi
}

# Test ping connectivity
test_ping() {
    log_info "Testing ping connectivity..."
    
    if ping -c 1 -W "$TIMEOUT" "$METADATA_IP" > /dev/null 2>&1; then
        log_success "Ping to $METADATA_IP successful"
    else
        log_error "Ping to $METADATA_IP failed"
    fi
}

# Test specific endpoints
test_endpoints() {
    log_info "Testing metadata endpoints..."
    
    local endpoints=(
        "/openstack/latest/meta_data.json"
        "/openstack/latest/network_data.json"
        "/openstack/latest/user_data"
    )
    
    for endpoint in "${endpoints[@]}"; do
        if curl -f -s --connect-timeout "$TIMEOUT" --max-time "$TIMEOUT" \
            "http://$METADATA_IP$endpoint" > /dev/null 2>&1; then
            log_success "Endpoint $endpoint is accessible"
        else
            log_error "Endpoint $endpoint is not accessible"
        fi
    done
}

# Test from different network contexts
test_network_contexts() {
    log_info "Testing from different network contexts..."
    
    # Test from host network namespace
    if ip netns list > /dev/null 2>&1; then
        log_info "Network namespaces available, testing contexts..."
        
        # This would require existing network namespaces
        # For now, just report that the feature is available
        log_success "Network namespace testing capability available"
    else
        log_warning "Network namespaces not available or insufficient permissions"
    fi
}

# Test BGP routing (if BIRD is installed)
test_bgp_routing() {
    log_info "Testing BGP routing configuration..."
    
    if command -v birdc > /dev/null 2>&1; then
        if birdc show route "$METADATA_IP/32" > /dev/null 2>&1; then
            local route_info
            route_info=$(birdc show route "$METADATA_IP/32" 2>/dev/null | grep -v "^BIRD")
            if [[ -n "$route_info" ]]; then
                log_success "BGP route for $METADATA_IP/32 found"
            else
                log_warning "BGP configured but no route for $METADATA_IP/32"
            fi
        else
            log_warning "BIRD is installed but not responding"
        fi
    else
        log_info "BIRD not installed - skipping BGP tests"
    fi
}

# Test keepalived status
test_keepalived() {
    log_info "Testing keepalived configuration..."
    
    if systemctl is-active keepalived > /dev/null 2>&1; then
        log_success "Keepalived service is active"
        
        # Check VRRP state
        if journalctl -u keepalived --since "1 minute ago" -q --no-pager | \
           grep -q "Entering MASTER STATE"; then
            log_success "Keepalived in MASTER state"
        elif journalctl -u keepalived --since "1 minute ago" -q --no-pager | \
             grep -q "Entering BACKUP STATE"; then
            log_success "Keepalived in BACKUP state"
        else
            log_warning "Keepalived state unclear from recent logs"
        fi
    else
        log_info "Keepalived not running - skipping HA tests"
    fi
}

# Test HAProxy status
test_haproxy() {
    log_info "Testing HAProxy configuration..."
    
    if systemctl is-active haproxy > /dev/null 2>&1; then
        log_success "HAProxy service is active"
        
        # Check if HAProxy is listening on metadata IP
        if netstat -ln 2>/dev/null | grep -q "$METADATA_IP:$METADATA_PORT" || \
           ss -ln 2>/dev/null | grep -q "$METADATA_IP:$METADATA_PORT"; then
            log_success "HAProxy listening on $METADATA_IP:$METADATA_PORT"
        else
            log_warning "HAProxy running but not listening on expected address"
        fi
    else
        log_info "HAProxy not running - skipping load balancer tests"
    fi
}

# Test Docker container status
test_docker_container() {
    log_info "Testing Docker container status..."
    
    if command -v docker > /dev/null 2>&1; then
        if docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "ironic-metadata"; then
            log_success "Docker container ironic-metadata is running"
        else
            log_warning "Docker container ironic-metadata not found or not running"
        fi
    else
        log_info "Docker not available - skipping container tests"
    fi
}

# Test firewall rules
test_firewall_rules() {
    log_info "Testing firewall configuration..."
    
    if command -v iptables > /dev/null 2>&1; then
        # Check for DNAT rules
        if iptables -t nat -L PREROUTING -n | grep -q "$METADATA_IP"; then
            log_success "DNAT rules found for metadata IP"
        else
            log_info "No DNAT rules found (may not be needed)"
        fi
        
        # Check for ACCEPT rules
        if iptables -L INPUT -n | grep -q "$METADATA_IP" || \
           iptables -L FORWARD -n | grep -q "$METADATA_IP"; then
            log_success "Firewall rules found for metadata traffic"
        else
            log_warning "No specific firewall rules found for metadata traffic"
        fi
    else
        log_info "iptables not available - skipping firewall tests"
    fi
}

# Test performance and latency
test_performance() {
    log_info "Testing performance and latency..."
    
    local response_time
    response_time=$(curl -o /dev/null -s -w "%{time_total}" \
        --connect-timeout "$TIMEOUT" --max-time "$TIMEOUT" \
        "http://$METADATA_IP/openstack/latest/meta_data.json" 2>/dev/null || echo "timeout")
    
    if [[ "$response_time" != "timeout" ]]; then
        local response_ms
        response_ms=$(echo "$response_time * 1000" | bc -l 2>/dev/null || echo "N/A")
        log_success "Response time: ${response_ms}ms"
        
        # Warn if response time is high
        if (( $(echo "$response_time > 1.0" | bc -l 2>/dev/null || echo 0) )); then
            log_warning "High response time detected (>1s)"
        fi
    else
        log_error "Performance test timed out"
    fi
}

# Run comprehensive validation
run_validation() {
    echo "======================================"
    echo "Ironic Metadata Advanced Network Tests"
    echo "======================================"
    echo
    
    # Core connectivity tests
    test_basic_connectivity
    test_ping
    test_ip_configuration
    test_routing
    test_service_listening
    test_endpoints
    
    echo
    log_info "Advanced configuration tests..."
    
    # Advanced configuration tests
    test_bgp_routing
    test_keepalived
    test_haproxy
    test_docker_container
    test_firewall_rules
    
    echo
    log_info "Additional tests..."
    
    # Additional tests
    test_dns_resolution
    test_network_contexts
    test_performance
    
    echo
    echo "======================================"
    echo "Test Summary"
    echo "======================================"
    echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "\n${GREEN}All critical tests passed!${NC}"
        exit 0
    else
        echo -e "\n${YELLOW}Some tests failed. Check configuration.${NC}"
        exit 1
    fi
}

# Show usage
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Options:
    --quick         Run only basic connectivity tests
    --help          Show this help message

Examples:
    $0              Run full validation suite
    $0 --quick      Run basic tests only
EOF
}

# Main function
main() {
    case "${1:-full}" in
        --quick)
            echo "Running quick validation tests..."
            test_basic_connectivity
            test_ping
            test_endpoints
            echo -e "\nQuick tests completed. Tests passed: ${GREEN}$TESTS_PASSED${NC}, failed: ${RED}$TESTS_FAILED${NC}"
            ;;
        --help|-h)
            show_usage
            ;;
        full|*)
            run_validation
            ;;
    esac
}

# Check if required tools are available
check_dependencies() {
    local missing_tools=()
    
    for tool in curl ping netstat ip; do
        if ! command -v "$tool" > /dev/null 2>&1; then
            missing_tools+=("$tool")
        fi
    done
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools and try again"
        exit 1
    fi
}

# Run dependency check and main function
check_dependencies
main "$@"

#!/bin/bash
#
# Advanced Network Configuration Script for Ironic Metadata Service
# This script helps configure various networking scenarios for global routing
#

set -euo pipefail

METADATA_IP="169.254.169.254"
METADATA_PORT="80"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root for network configuration"
        exit 1
    fi
}

# Detect the operating system
detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        VER=$VERSION_ID
    else
        log_error "Cannot detect operating system"
        exit 1
    fi
    log_info "Detected OS: $OS $VER"
}

# Get the primary network interface
get_primary_interface() {
    local interface
    interface=$(ip route | grep default | head -n1 | awk '{print $5}')
    if [[ -z "$interface" ]]; then
        log_error "Could not detect primary network interface"
        exit 1
    fi
    echo "$interface"
}

# Get the host IP address
get_host_ip() {
    local interface="$1"
    local ip
    ip=$(ip addr show "$interface" | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d/ -f1)
    if [[ -z "$ip" ]]; then
        log_error "Could not get IP address for interface $interface"
        exit 1
    fi
    echo "$ip"
}

# Setup NAT routing
setup_nat_routing() {
    local host_ip="$1"
    
    log_info "Setting up NAT routing for metadata service"
    
    # Enable IP forwarding
    echo 1 > /proc/sys/net/ipv4/ip_forward
    
    # Make IP forwarding persistent
    if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf; then
        echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
    fi
    
    # Configure iptables rules
    # DNAT for incoming traffic to metadata IP
    iptables -t nat -C PREROUTING -d "$METADATA_IP" -p tcp --dport "$METADATA_PORT" \
        -j DNAT --to-destination "$host_ip:$METADATA_PORT" 2>/dev/null || \
    iptables -t nat -A PREROUTING -d "$METADATA_IP" -p tcp --dport "$METADATA_PORT" \
        -j DNAT --to-destination "$host_ip:$METADATA_PORT"
    
    # SNAT for return traffic
    iptables -t nat -C POSTROUTING -s "$host_ip" -p tcp --sport "$METADATA_PORT" \
        -j SNAT --to-source "$METADATA_IP" 2>/dev/null || \
    iptables -t nat -A POSTROUTING -s "$host_ip" -p tcp --sport "$METADATA_PORT" \
        -j SNAT --to-source "$METADATA_IP"
    
    # Allow forwarding
    iptables -C FORWARD -d "$host_ip" -p tcp --dport "$METADATA_PORT" -j ACCEPT 2>/dev/null || \
    iptables -A FORWARD -d "$host_ip" -p tcp --dport "$METADATA_PORT" -j ACCEPT
    
    iptables -C FORWARD -s "$host_ip" -p tcp --sport "$METADATA_PORT" -j ACCEPT 2>/dev/null || \
    iptables -A FORWARD -s "$host_ip" -p tcp --sport "$METADATA_PORT" -j ACCEPT
    
    # Save iptables rules
    if command -v iptables-save &> /dev/null; then
        case "$OS" in
            ubuntu|debian)
                iptables-save > /etc/iptables/rules.v4 2>/dev/null || \
                mkdir -p /etc/iptables && iptables-save > /etc/iptables/rules.v4
                ;;
            centos|rhel|fedora)
                iptables-save > /etc/sysconfig/iptables
                ;;
        esac
    fi
    
    log_success "NAT routing configured"
}

# Setup BGP routing with BIRD
setup_bgp_bird() {
    local router_id="$1"
    local local_as="$2"
    local peer_ip="$3"
    local peer_as="$4"
    
    log_info "Setting up BGP routing with BIRD"
    
    # Install BIRD if not present
    case "$OS" in
        ubuntu|debian)
            apt-get update
            apt-get install -y bird
            ;;
        centos|rhel|fedora)
            if command -v dnf &> /dev/null; then
                dnf install -y bird
            else
                yum install -y bird
            fi
            ;;
    esac
    
    # Configure loopback interface for anycast
    ip link add name lo-metadata type dummy 2>/dev/null || true
    ip addr add "$METADATA_IP/32" dev lo-metadata 2>/dev/null || true
    ip link set lo-metadata up
    
    # Create BIRD configuration
    cat > /etc/bird.conf << EOF
# BIRD configuration for metadata service anycast
router id $router_id;

# Configure logging
log syslog all;

# Define static routes for metadata service
protocol static metadata_routes {
    route $METADATA_IP/32 via "lo-metadata";
}

# Configure BGP
protocol bgp metadata_bgp {
    local as $local_as;
    neighbor $peer_ip as $peer_as;
    
    export filter {
        if net = $METADATA_IP/32 then accept;
        reject;
    };
    
    import none;
}
EOF
    
    # Enable and start BIRD
    systemctl enable bird
    systemctl restart bird
    
    log_success "BGP routing with BIRD configured"
}

# Setup keepalived for high availability
setup_keepalived() {
    local interface="$1"
    local priority="${2:-100}"
    local auth_pass="${3:-changeme}"
    
    log_info "Setting up keepalived for high availability"
    
    # Install keepalived
    case "$OS" in
        ubuntu|debian)
            apt-get update
            apt-get install -y keepalived
            ;;
        centos|rhel|fedora)
            if command -v dnf &> /dev/null; then
                dnf install -y keepalived
            else
                yum install -y keepalived
            fi
            ;;
    esac
    
    # Create health check script
    cat > /usr/local/bin/check_metadata.sh << 'EOF'
#!/bin/bash
# Health check script for metadata service

curl -f -s --connect-timeout 5 --max-time 10 \
    http://169.254.169.254/openstack/latest/meta_data.json > /dev/null

exit $?
EOF
    chmod +x /usr/local/bin/check_metadata.sh
    
    # Create keepalived configuration
    cat > /etc/keepalived/keepalived.conf << EOF
# Keepalived configuration for metadata service HA

vrrp_script chk_metadata {
    script "/usr/local/bin/check_metadata.sh"
    interval 5
    timeout 3
    weight -2
    fall 3
    rise 2
}

vrrp_instance VI_METADATA {
    state MASTER
    interface $interface
    virtual_router_id 51
    priority $priority
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass $auth_pass
    }
    virtual_ipaddress {
        $METADATA_IP/32
    }
    track_script {
        chk_metadata
    }
    notify_master "/usr/local/bin/metadata_master.sh"
    notify_backup "/usr/local/bin/metadata_backup.sh"
    notify_fault "/usr/local/bin/metadata_fault.sh"
}
EOF
    
    # Create notification scripts
    cat > /usr/local/bin/metadata_master.sh << 'EOF'
#!/bin/bash
logger "Metadata service: Transitioned to MASTER state"
# Start metadata service if not running
docker-compose -f /opt/ironic-metadata/docker-compose.yml up -d ironic-metadata
EOF
    
    cat > /usr/local/bin/metadata_backup.sh << 'EOF'
#!/bin/bash
logger "Metadata service: Transitioned to BACKUP state"
# Stop metadata service
docker-compose -f /opt/ironic-metadata/docker-compose.yml stop ironic-metadata
EOF
    
    cat > /usr/local/bin/metadata_fault.sh << 'EOF'
#!/bin/bash
logger "Metadata service: Entered FAULT state"
# Restart metadata service
docker-compose -f /opt/ironic-metadata/docker-compose.yml restart ironic-metadata
EOF
    
    chmod +x /usr/local/bin/metadata_*.sh
    
    # Enable and start keepalived
    systemctl enable keepalived
    systemctl restart keepalived
    
    log_success "Keepalived configured for high availability"
}

# Setup HAProxy load balancer
setup_haproxy() {
    local backend_servers="$1"  # Comma-separated list of IP:PORT
    
    log_info "Setting up HAProxy load balancer"
    
    # Install HAProxy
    case "$OS" in
        ubuntu|debian)
            apt-get update
            apt-get install -y haproxy
            ;;
        centos|rhel|fedora)
            if command -v dnf &> /dev/null; then
                dnf install -y haproxy
            else
                yum install -y haproxy
            fi
            ;;
    esac
    
    # Backup original configuration
    cp /etc/haproxy/haproxy.cfg /etc/haproxy/haproxy.cfg.backup
    
    # Create HAProxy configuration
    cat > /etc/haproxy/haproxy.cfg << EOF
global
    daemon
    log 127.0.0.1:514 local0
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy

defaults
    mode http
    log global
    option httplog
    option dontlognull
    option log-health-checks
    timeout connect 5s
    timeout client 30s
    timeout server 30s
    errorfile 400 /etc/haproxy/errors/400.http
    errorfile 403 /etc/haproxy/errors/403.http
    errorfile 408 /etc/haproxy/errors/408.http
    errorfile 500 /etc/haproxy/errors/500.http
    errorfile 502 /etc/haproxy/errors/502.http
    errorfile 503 /etc/haproxy/errors/503.http
    errorfile 504 /etc/haproxy/errors/504.http

frontend metadata_frontend
    bind $METADATA_IP:$METADATA_PORT
    default_backend metadata_backend

backend metadata_backend
    balance roundrobin
    option httpchk GET /openstack/latest/meta_data.json
    http-check expect status 200
EOF
    
    # Add backend servers
    local counter=1
    IFS=',' read -ra SERVERS <<< "$backend_servers"
    for server in "${SERVERS[@]}"; do
        echo "    server metadata$counter $server check" >> /etc/haproxy/haproxy.cfg
        ((counter++))
    done
    
    # Enable and start HAProxy
    systemctl enable haproxy
    systemctl restart haproxy
    
    log_success "HAProxy load balancer configured"
}

# Test metadata service connectivity
test_connectivity() {
    log_info "Testing metadata service connectivity"
    
    # Test local connectivity
    if curl -f -s --connect-timeout 5 "http://$METADATA_IP/openstack/latest/meta_data.json" > /dev/null; then
        log_success "Metadata service is responding locally"
    else
        log_error "Metadata service is not responding locally"
        return 1
    fi
    
    # Test routing
    log_info "Route to metadata service:"
    ip route get "$METADATA_IP" || log_warning "Could not get route to metadata service"
    
    # Test if service is listening
    if netstat -ln | grep -q ":$METADATA_PORT.*$METADATA_IP"; then
        log_success "Service is listening on $METADATA_IP:$METADATA_PORT"
    else
        log_warning "Service may not be listening on expected address"
    fi
}

# Show usage information
show_usage() {
    cat << EOF
Usage: $0 [COMMAND] [OPTIONS]

Commands:
    nat-routing [HOST_IP]          Setup NAT routing to redirect traffic
    bgp-bird ROUTER_ID LOCAL_AS PEER_IP PEER_AS
                                   Setup BGP routing with BIRD
    keepalived [INTERFACE] [PRIORITY] [AUTH_PASS]
                                   Setup keepalived for HA
    haproxy BACKEND_SERVERS        Setup HAProxy load balancer
                                   (BACKEND_SERVERS: comma-separated IP:PORT list)
    test                           Test metadata service connectivity
    help                           Show this help message

Examples:
    $0 nat-routing 192.168.1.100
    $0 bgp-bird 1.1.1.1 65001 192.168.1.1 65000
    $0 keepalived eth0 100 secure_password
    $0 haproxy "192.168.1.10:80,192.168.1.11:80,192.168.1.12:80"
    $0 test
EOF
}

# Main function
main() {
    case "${1:-help}" in
        nat-routing)
            check_root
            detect_os
            local host_ip="${2:-$(get_host_ip "$(get_primary_interface)")}"
            setup_nat_routing "$host_ip"
            test_connectivity
            ;;
        bgp-bird)
            if [[ $# -ne 5 ]]; then
                log_error "BGP setup requires: ROUTER_ID LOCAL_AS PEER_IP PEER_AS"
                show_usage
                exit 1
            fi
            check_root
            detect_os
            setup_bgp_bird "$2" "$3" "$4" "$5"
            test_connectivity
            ;;
        keepalived)
            check_root
            detect_os
            local interface="${2:-$(get_primary_interface)}"
            local priority="${3:-100}"
            local auth_pass="${4:-changeme}"
            setup_keepalived "$interface" "$priority" "$auth_pass"
            ;;
        haproxy)
            if [[ $# -ne 2 ]]; then
                log_error "HAProxy setup requires backend servers list"
                show_usage
                exit 1
            fi
            check_root
            detect_os
            setup_haproxy "$2"
            test_connectivity
            ;;
        test)
            test_connectivity
            ;;
        help|--help|-h)
            show_usage
            ;;
        *)
            log_error "Unknown command: $1"
            show_usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"

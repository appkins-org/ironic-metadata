# Advanced Networking Guide

This guide covers advanced networking scenarios for making the ironic-metadata service globally routable and highly available.

## Overview

The metadata service needs to be accessible at `169.254.169.254` from compute nodes across your infrastructure. This document covers various approaches to achieve global reachability beyond the local network segment.

## Architecture Patterns

### 1. Centralized Metadata Service

Single metadata service instance with global routing:

```text
[Compute Nodes] --> [Router/Gateway] --> [Metadata Service Host]
     |                     |                        |
     |                     |                        |
  Request to          NAT/Proxy              Docker Container
169.254.169.254      169.254.169.254      169.254.169.254:80
```

### 2. Distributed Metadata Services

Multiple instances with anycast routing:

```text
[Compute Nodes] --> [Network Fabric] --> [Metadata Service 1]
     |                     |          --> [Metadata Service 2]
     |                     |          --> [Metadata Service 3]
     |                     |
  Request to         BGP Anycast
169.254.169.254    169.254.169.254/32
```

### 3. Proxy-Based Distribution

Central proxy with backend metadata services:

```text
[Compute Nodes] --> [Load Balancer] --> [Backend Services]
     |                     |                    |
     |                     |                    |
  Request to         Proxy/LB            Multiple Instances
169.254.169.254    169.254.169.254      Different Ports/Hosts
```

## Implementation Methods

### Method 1: Router-Based DNAT

Configure your network infrastructure to redirect traffic:

#### Cisco Router Configuration

```cisco
! Configure NAT for metadata service
ip nat inside source static 169.254.169.254 <INTERNAL_HOST_IP>
ip nat outside source static <INTERNAL_HOST_IP> 169.254.169.254

! Apply to interfaces
interface GigabitEthernet0/0
 ip nat outside
interface GigabitEthernet0/1
 ip nat inside
```

#### Linux iptables Configuration

```bash
#!/bin/bash
# /etc/ironic-metadata/setup-routing.sh

# Enable IP forwarding
echo 1 > /proc/sys/net/ipv4/ip_forward

# Configure DNAT for incoming traffic
iptables -t nat -A PREROUTING -d 169.254.169.254 -p tcp --dport 80 \
  -j DNAT --to-destination <METADATA_HOST_IP>:80

# Configure SNAT for return traffic
iptables -t nat -A POSTROUTING -s <METADATA_HOST_IP> -p tcp --sport 80 \
  -j SNAT --to-source 169.254.169.254

# Save rules
iptables-save > /etc/iptables/rules.v4
```

#### pfSense/OPNsense Configuration

```xml
<!-- Port Forward Rule -->
<rule>
  <interface>wan</interface>
  <protocol>tcp</protocol>
  <source>
    <any>1</any>
  </source>
  <destination>
    <address>169.254.169.254</address>
    <port>80</port>
  </destination>
  <target>
    <address>METADATA_HOST_IP</address>
    <port>80</port>
  </target>
  <descr>Ironic Metadata Service</descr>
</rule>
```

### Method 2: BGP Anycast Routing

Deploy metadata services with BGP route announcement:

#### BIRD Configuration

```bird
# /etc/bird/bird.conf
router id <ROUTER_ID>;

protocol static metadata_routes {
    route 169.254.169.254/32 via "lo";
}

protocol bgp metadata_bgp {
    local as <LOCAL_AS>;
    neighbor <BGP_PEER_IP> as <PEER_AS>;
    export filter {
        if net = 169.254.169.254/32 then accept;
        reject;
    };
}
```

#### FRRouting Configuration

```frr
# /etc/frr/frr.conf
hostname metadata-server
!
router bgp <AS_NUMBER>
 bgp router-id <ROUTER_ID>
 network 169.254.169.254/32
 neighbor <PEER_IP> remote-as <PEER_AS>
 neighbor <PEER_IP> soft-reconfiguration inbound
!
ip route 169.254.169.254/32 lo
!
```

#### Loopback Interface Setup

```bash
#!/bin/bash
# Configure loopback interface for anycast

# Add loopback interface
ip link add name lo:metadata type dummy
ip addr add 169.254.169.254/32 dev lo:metadata
ip link set lo:metadata up

# Add to startup
cat > /etc/systemd/network/lo-metadata.netdev << EOF
[NetDev]
Name=lo-metadata
Kind=dummy
EOF

cat > /etc/systemd/network/lo-metadata.network << EOF
[Match]
Name=lo-metadata

[Network]
Address=169.254.169.254/32
EOF
```

### Method 3: Load Balancer/Proxy Setup

#### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    daemon
    log 127.0.0.1:514 local0

defaults
    mode http
    timeout connect 5s
    timeout client 30s
    timeout server 30s

frontend metadata_frontend
    bind 169.254.169.254:80
    default_backend metadata_backend

backend metadata_backend
    balance roundrobin
    option httpchk GET /openstack/latest/meta_data.json
    server metadata1 <HOST1_IP>:80 check
    server metadata2 <HOST2_IP>:80 check
    server metadata3 <HOST3_IP>:80 check
```

#### Nginx Configuration

```nginx
# /etc/nginx/sites-available/metadata
upstream metadata_backend {
    least_conn;
    server <HOST1_IP>:80 max_fails=3 fail_timeout=30s;
    server <HOST2_IP>:80 max_fails=3 fail_timeout=30s;
    server <HOST3_IP>:80 max_fails=3 fail_timeout=30s;
}

server {
    listen 169.254.169.254:80;
    server_name 169.254.169.254;

    location /openstack/latest/ {
        proxy_pass http://metadata_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    # Health check endpoint
    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }
}
```

### Method 4: Software-Defined Networking

#### OpenStack Neutron

```bash
# Create a dedicated network for metadata routing
openstack network create --provider-network-type vlan \
  --provider-physical-network physnet1 \
  --provider-segment 100 metadata-routing

# Create subnet
openstack subnet create --network metadata-routing \
  --subnet-range 169.254.169.0/24 \
  --allocation-pool start=169.254.169.254,end=169.254.169.254 \
  --no-dhcp metadata-subnet

# Add static route to routers
openstack router set --route destination=169.254.169.254/32,gateway=<METADATA_HOST> \
  <ROUTER_ID>
```

#### VMware NSX-T

```json
{
  "resource_type": "StaticRoute",
  "display_name": "metadata-route",
  "network": "169.254.169.254/32",
  "next_hops": [
    {
      "ip_address": "<METADATA_HOST_IP>",
      "admin_distance": 1
    }
  ]
}
```

## High Availability Patterns

### Active/Passive with Keepalived

```bash
# /etc/keepalived/keepalived.conf - Master
vrrp_script chk_metadata {
    script "/usr/local/bin/check_metadata.sh"
    interval 5
    weight -2
    fall 3
    rise 2
}

vrrp_instance VI_METADATA {
    state MASTER
    interface eth0
    virtual_router_id 51
    priority 100
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass secure_password
    }
    virtual_ipaddress {
        169.254.169.254/32
    }
    track_script {
        chk_metadata
    }
    notify_master "/usr/local/bin/metadata_master.sh"
    notify_backup "/usr/local/bin/metadata_backup.sh"
}
```

### Health Check Script

```bash
#!/bin/bash
# /usr/local/bin/check_metadata.sh

# Check if metadata service is responding
curl -f -s --connect-timeout 5 \
  http://169.254.169.254/openstack/latest/meta_data.json > /dev/null

if [ $? -eq 0 ]; then
    exit 0
else
    exit 1
fi
```

### Docker Compose HA Setup

```yaml
# docker-compose.ha.yml
version: '3.8'

services:
  ironic-metadata-primary:
    image: ironic-metadata:latest
    environment:
      - ROLE=primary
      - BIND_ADDR=169.254.169.254
      - BIND_PORT=80
    networks:
      metadata-network:
        ipv4_address: 169.254.169.254
    volumes:
      - /etc/keepalived:/etc/keepalived:ro
    cap_add:
      - NET_ADMIN
    privileged: true

  ironic-metadata-secondary:
    image: ironic-metadata:latest
    environment:
      - ROLE=secondary
      - BIND_ADDR=169.254.169.253
      - BIND_PORT=80
    networks:
      metadata-network:
        ipv4_address: 169.254.169.253
    volumes:
      - /etc/keepalived:/etc/keepalived:ro
    cap_add:
      - NET_ADMIN
    privileged: true

networks:
  metadata-network:
    driver: macvlan
    driver_opts:
      parent: ${NETWORK_INTERFACE:-eth0}
    ipam:
      config:
        - subnet: 169.254.169.0/24
          ip_range: 169.254.169.252/30
          gateway: 169.254.169.1
```

## Cloud Provider Specific Configurations

### AWS VPC

```bash
# Create custom route table
aws ec2 create-route-table --vpc-id vpc-12345678

# Add route for metadata service
aws ec2 create-route --route-table-id rtb-12345678 \
  --destination-cidr-block 169.254.169.254/32 \
  --instance-id i-metadata-server

# Associate with subnets
aws ec2 associate-route-table --subnet-id subnet-12345678 \
  --route-table-id rtb-12345678
```

### Google Cloud Platform

```bash
# Create custom route
gcloud compute routes create metadata-route \
  --destination-range=169.254.169.254/32 \
  --next-hop-instance=metadata-server \
  --next-hop-instance-zone=us-central1-a
```

### Azure

```bash
# Create route table
az network route-table create --resource-group myResourceGroup \
  --name MetadataRouteTable

# Add route
az network route-table route create --resource-group myResourceGroup \
  --route-table-name MetadataRouteTable --name MetadataRoute \
  --address-prefix 169.254.169.254/32 --next-hop-type VirtualAppliance \
  --next-hop-ip-address <METADATA_SERVER_IP>
```

## Monitoring and Troubleshooting

### Network Diagnostics

```bash
#!/bin/bash
# /usr/local/bin/diagnose-metadata-routing.sh

echo "=== Metadata Service Network Diagnostics ==="

# Check if metadata IP is configured
ip addr show | grep -q 169.254.169.254
if [ $? -eq 0 ]; then
    echo "✓ Metadata IP configured locally"
else
    echo "✗ Metadata IP not configured locally"
fi

# Check routing
echo "Route to metadata service:"
ip route get 169.254.169.254

# Check if service is listening
netstat -ln | grep -q ":80.*169.254.169.254"
if [ $? -eq 0 ]; then
    echo "✓ Service listening on 169.254.169.254:80"
else
    echo "✗ Service not listening on expected address"
fi

# Test connectivity
curl -s --connect-timeout 5 http://169.254.169.254/openstack/latest/meta_data.json > /dev/null
if [ $? -eq 0 ]; then
    echo "✓ Metadata service responding"
else
    echo "✗ Metadata service not responding"
fi

# Check BGP routes (if using BGP)
if command -v birdc &> /dev/null; then
    echo "BGP routes for metadata:"
    birdc show route 169.254.169.254/32
fi
```

### Performance Monitoring

```bash
#!/bin/bash
# /usr/local/bin/monitor-metadata-performance.sh

# Monitor response times
while true; do
    response_time=$(curl -o /dev/null -s -w "%{time_total}" \
      http://169.254.169.254/openstack/latest/meta_data.json)
    echo "$(date): Response time: ${response_time}s"
    sleep 10
done
```

### Log Analysis

```bash
# Monitor access patterns
tail -f /var/log/nginx/access.log | grep 169.254.169.254

# Analyze traffic patterns
tcpdump -i any -n host 169.254.169.254 and port 80

# Monitor for routing changes
ip monitor route | grep 169.254.169.254
```

## Security Considerations

### Access Control

```bash
# Restrict access to metadata service
iptables -A INPUT -d 169.254.169.254 -p tcp --dport 80 \
  -s <ALLOWED_NETWORK>/24 -j ACCEPT
iptables -A INPUT -d 169.254.169.254 -p tcp --dport 80 -j DROP
```

### Rate Limiting

```nginx
# Nginx rate limiting
http {
    limit_req_zone $binary_remote_addr zone=metadata:10m rate=10r/s;
    
    server {
        listen 169.254.169.254:80;
        limit_req zone=metadata burst=20 nodelay;
        # ... rest of configuration
    }
}
```

### Monitoring and Alerting

```yaml
# Prometheus alerting rules
groups:
- name: metadata-service
  rules:
  - alert: MetadataServiceDown
    expr: up{job="metadata-service"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Metadata service is down"
      
  - alert: MetadataHighLatency
    expr: http_request_duration_seconds{job="metadata-service"} > 1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Metadata service high latency"
```

This guide provides comprehensive approaches for making your ironic-metadata service globally routable and highly available across different network architectures and cloud environments.

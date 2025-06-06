# Docker Deployment with Macvlan

This configuration deploys the ironic-metadata service using Docker with a macvlan network to bind directly to the IP address `169.254.169.254`.

## Prerequisites

1. **Docker and Docker Compose** installed on the target host
2. **Network interface access** - the container needs to attach to the host's network interface
3. **IP availability** - ensure `169.254.169.254` is not already in use on your network

## Quick Setup

1. **Configure the network interface**:

   ```bash
   # Auto-detect and configure the network interface
   ./scripts/setup-network.sh
   ```

   Or manually edit `docker-compose.yml` and change the `parent` field under `metadata-network` to match your host's primary network interface:

   ```bash
   # Find your network interface
   ip route | grep default
   # or
   ip link show
   ```

2. **Deploy the service**:

   ```bash
   docker-compose up -d ironic-metadata
   ```

3. **Verify the deployment**:

   ```bash
   # Check if the container is running
   docker-compose ps
   
   # Test the metadata service
   curl http://169.254.169.254/openstack/latest/meta_data.json
   ```

## Configuration

The service connects to your remote Ironic API at `https://ironic.appkins.io/v1` as configured in the environment variables.

### Environment Variables

- `IRONIC_URL`: The URL of your Ironic API endpoint (default: <https://ironic.appkins.io/v1>)
- `BIND_ADDR`: The IP address to bind to (set to 169.254.169.254 for metadata service)
- `BIND_PORT`: The port to bind to (default: 80)
- `LOG_LEVEL`: Logging level (default: info)

### Network Configuration

The macvlan network configuration:

- **Subnet**: `169.254.169.0/24` (link-local address range)
- **IP Range**: `169.254.169.254/32` (single IP for metadata service)
- **Gateway**: `169.254.169.1`

## Troubleshooting

### Common Issues

1. **"network not found" error**:
   - Ensure the parent network interface name is correct in docker-compose.yml
   - Run `./scripts/setup-network.sh` to auto-detect the interface

2. **"address already in use" error**:
   - Check if another service is using 169.254.169.254
   - Verify no conflicts with existing network configuration

3. **Connection refused to Ironic API**:
   - Verify the IRONIC_URL is correct and accessible from the container
   - Check network connectivity from the Docker host to the Ironic API

### Logs

Check service logs:

```bash
docker-compose logs ironic-metadata
```

### Manual Testing

Test individual components:

```bash
# Test from inside the container
docker-compose exec ironic-metadata curl http://169.254.169.254/openstack/latest/meta_data.json

# Test the Ironic API connection
docker-compose exec ironic-metadata curl https://ironic.appkins.io/v1/
```

## Making 169.254.169.254 Globally Routable

The default macvlan configuration only makes 169.254.169.254 accessible on the local network segment. To make it globally routable/addressable, you have several options:

### Option 1: Router-Based NAT/DNAT (Recommended)

Configure your network router or firewall to perform Destination NAT (DNAT) to forward traffic from the global network to your metadata service:

```bash
# Example iptables rule on your router/gateway
iptables -t nat -A PREROUTING -d 169.254.169.254 -j DNAT --to-destination <HOST_IP>:80

# For pfSense/OPNsense, configure Port Forward rules:
# WAN Interface -> 169.254.169.254:80 -> Internal Host IP:80
```

### Option 2: BGP Announcement

If you have control over your network's BGP routing:

```bash
# Announce the 169.254.169.254/32 route via BGP
# This requires BGP daemon configuration (e.g., BIRD, FRR)

# Example BIRD configuration snippet:
protocol static {
    route 169.254.169.254/32 via <HOST_IP>;
}
```

### Option 3: Anycast Setup

For high availability across multiple sites:

```bash
# Deploy identical metadata services on multiple hosts
# Configure each to announce 169.254.169.254/32 via IGP/BGP
# Network will route to the nearest/best instance
```

### Option 4: Proxy/Load Balancer

Use a reverse proxy or load balancer to forward traffic:

```nginx
# Nginx configuration
server {
    listen 169.254.169.254:80;
    server_name 169.254.169.254;
    
    location / {
        proxy_pass http://<BACKEND_HOST>:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### Option 5: DHCP Option 121 (Static Routes)

Configure your DHCP server to push static routes to clients:

```bash
# DHCP Option 121 configuration
# Pushes route: 169.254.169.254/32 via <HOST_IP>
option rfc3442-classless-static-routes code 121 = array of unsigned integer 8;
option rfc3442-classless-static-routes 32, 169, 254, 169, 254, <HOST_IP_OCTETS>;
```

### Option 6: Software-Defined Networking (SDN)

For OpenStack/VMware environments:

```yaml
# OpenStack Neutron router configuration
openstack router set --route destination=169.254.169.254/32,gateway=<HOST_IP> <ROUTER_ID>

# VMware NSX-T configuration
# Create static route for 169.254.169.254/32 -> <HOST_IP>
```

## Advanced Networking Scenarios

### Multi-Site Deployment

For geographically distributed deployments:

```yaml
# docker-compose.override.yml for site-specific configuration
version: '3.8'
services:
  ironic-metadata:
    environment:
      - SITE_ID=site1
      - IRONIC_URL=https://ironic-site1.example.com/v1
    networks:
      metadata-network:
        ipv4_address: 169.254.169.254
```

### High Availability Setup

```bash
# Use keepalived for IP failover
# /etc/keepalived/keepalived.conf
vrrp_instance VI_METADATA {
    state MASTER
    interface eth0
    virtual_router_id 51
    priority 100
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass changeme
    }
    virtual_ipaddress {
        169.254.169.254/32
    }
}
```

### Container Orchestration

For Kubernetes deployment with global routing:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ironic-metadata
  annotations:
    metallb.universe.tf/address-pool: metadata-pool
spec:
  type: LoadBalancer
  loadBalancerIP: 169.254.169.254
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: ironic-metadata
```

## Network Verification

### Test Global Reachability

```bash
# From a remote network segment
ping 169.254.169.254

# Test HTTP connectivity
curl -v http://169.254.169.254/openstack/latest/meta_data.json

# Trace routing path
traceroute 169.254.169.254
```

### Monitor Traffic

```bash
# Monitor traffic to the metadata service
tcpdump -i any host 169.254.169.254

# Check routing table
ip route get 169.254.169.254

# Verify BGP announcements (if using BGP)
birdc show route 169.254.169.254/32
```

## Security Considerations

- The macvlan network gives the container direct access to the host network
- Ensure proper firewall rules are in place to restrict access to the metadata service
- Consider using host networking mode as an alternative if macvlan is not suitable for your environment
- When making the service globally routable, implement proper access controls and rate limiting
- Monitor for unauthorized access attempts to the metadata endpoints
- Consider implementing IP whitelisting for known client networks

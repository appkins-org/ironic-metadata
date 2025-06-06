# Ironic Metadata Service - Advanced Networking Summary

This document provides a comprehensive overview of the advanced networking capabilities implemented for the ironic-metadata service, enabling global routing and high availability for the 169.254.169.254 metadata endpoint.

## Quick Start

### Basic Docker Deployment

```bash
# 1. Auto-configure network interface
./scripts/setup-network.sh

# 2. Start the service
docker-compose up -d ironic-metadata

# 3. Test connectivity
curl http://169.254.169.254/openstack/latest/meta_data.json
```

### Global Routing Setup

```bash
# Choose one method based on your infrastructure:

# Method 1: NAT/DNAT (most common)
sudo ./scripts/setup-advanced-networking.sh nat-routing

# Method 2: BGP Anycast (for distributed deployments)
sudo ./scripts/setup-advanced-networking.sh bgp-bird 1.1.1.1 65001 192.168.1.1 65000

# Method 3: High Availability (for redundancy)
sudo ./scripts/setup-advanced-networking.sh keepalived eth0 100

# Method 4: Load Balancing (for scale)
sudo ./scripts/setup-advanced-networking.sh haproxy "192.168.1.10:80,192.168.1.11:80"
```

### Validation

```bash
# Full network validation
./scripts/validate-networking.sh

# Quick connectivity test
./scripts/validate-networking.sh --quick
```

## Architecture Overview

The ironic-metadata service supports multiple deployment patterns:

### 1. Local Deployment (Development/Testing)

- Single Docker container with macvlan
- Direct binding to 169.254.169.254
- Suitable for local development and testing

### 2. NAT-based Routing (Production - Single Site)

- Router/firewall redirects traffic to metadata service
- Most common production deployment
- Easy to implement with existing network infrastructure

### 3. BGP Anycast (Production - Multi-Site)

- Multiple metadata service instances
- BGP announces the same route from different locations
- Automatic failover and load distribution
- Best for geographically distributed deployments

### 4. High Availability (Production - Redundancy)

- Active/passive configuration with keepalived
- Automatic failover on service failure
- Shared virtual IP address
- Health checks and notifications

### 5. Load Balanced (Production - Scale)

- Multiple backend instances
- HAProxy or similar load balancer
- Health checking and traffic distribution
- Horizontal scaling capability

## File Structure

```
ironic-metadata/
├── cmd/ironic-metadata/main.go          # Main service application
├── api/metadata/metadata.go             # HTTP handlers and routing
├── pkg/client/metadata.go               # Ironic client integration
├── docker-compose.yml                   # Docker deployment configuration
├── Dockerfile                           # Container build instructions
├── scripts/
│   ├── setup-network.sh               # Network interface auto-detection
│   ├── setup-advanced-networking.sh   # Advanced routing configuration
│   └── validate-networking.sh         # Network validation and testing
├── docs/
│   ├── DOCKER_DEPLOYMENT.md           # Docker/macvlan deployment guide
│   └── ADVANCED_NETWORKING.md         # Comprehensive networking guide
└── README.md                           # Main documentation
```

## Network Methods Comparison

| Method | Complexity | Availability | Scale | Use Case |
|--------|------------|-------------|-------|----------|
| Local (macvlan) | Low | Single point | Single host | Development, testing |
| NAT/DNAT | Medium | Single point | Single site | Small to medium production |
| BGP Anycast | High | High | Multi-site | Large distributed deployments |
| Keepalived HA | Medium | High | Single site | Critical single-site deployments |
| Load Balanced | Medium | High | High | High-traffic deployments |

## Configuration Examples

### Environment Variables

Core service configuration:

```bash
IRONIC_URL=https://ironic.example.com/v1    # Ironic API endpoint
BIND_ADDR=169.254.169.254                   # Metadata service IP
BIND_PORT=80                                # Service port
LOG_LEVEL=info                              # Logging level
```

### Docker Compose Variants

#### Basic Local Development

```yaml
# Uses default docker-compose.yml
docker-compose up -d ironic-metadata
```

#### Remote Ironic API

```yaml
services:
  ironic-metadata:
    environment:
      - IRONIC_URL=https://your-ironic-api.com/v1
    # Remove ironic-api and ironic-db services
```

#### High Availability

```yaml
services:
  ironic-metadata-primary:
    environment:
      - ROLE=primary
    volumes:
      - /etc/keepalived:/etc/keepalived:ro
    cap_add:
      - NET_ADMIN
```

## Monitoring and Troubleshooting

### Health Checks

```bash
# Service health
curl -f http://169.254.169.254/openstack/latest/meta_data.json

# Network connectivity
ping 169.254.169.254

# Routing verification
ip route get 169.254.169.254
traceroute 169.254.169.254
```

### Log Analysis

```bash
# Service logs
docker-compose logs ironic-metadata

# System logs
journalctl -u keepalived
journalctl -u haproxy

# Network traffic
tcpdump -i any host 169.254.169.254
```

### Performance Monitoring

```bash
# Response time monitoring
while true; do
  curl -o /dev/null -s -w "Response time: %{time_total}s\n" \
    http://169.254.169.254/openstack/latest/meta_data.json
  sleep 10
done
```

## Security Considerations

### Access Control

- Implement firewall rules to restrict access to authorized networks
- Use IP whitelisting for known client subnets
- Monitor for unauthorized access attempts

### Network Isolation

- Deploy metadata service in isolated network segments when possible
- Use VLANs or network namespaces for traffic separation
- Implement network ACLs at router/switch level

### Authentication and Authorization

- Secure Ironic API access with proper credentials
- Use TLS for Ironic API communication
- Regularly rotate service credentials

## Production Checklist

Before deploying to production:

- [ ] Choose appropriate network architecture for your environment
- [ ] Configure monitoring and alerting for service health
- [ ] Implement proper backup and disaster recovery procedures
- [ ] Set up log aggregation and analysis
- [ ] Configure firewall rules and access controls
- [ ] Test failover scenarios (if using HA configuration)
- [ ] Document network dependencies and troubleshooting procedures
- [ ] Set up performance monitoring and capacity planning
- [ ] Verify integration with existing network infrastructure
- [ ] Test from actual compute nodes in your environment

## Support and Documentation

### Primary Documentation

- [Docker Deployment Guide](DOCKER_DEPLOYMENT.md) - Basic Docker setup
- [Advanced Networking Guide](ADVANCED_NETWORKING.md) - Comprehensive networking
- [README.md](../README.md) - Service overview and API documentation

### Scripts and Tools

- `scripts/setup-network.sh` - Automatic network interface detection
- `scripts/setup-advanced-networking.sh` - Advanced routing configuration
- `scripts/validate-networking.sh` - Network validation and testing

### Common Commands

```bash
# Quick deployment
./scripts/setup-network.sh && docker-compose up -d

# Full validation
./scripts/validate-networking.sh

# Advanced routing setup
sudo ./scripts/setup-advanced-networking.sh nat-routing

# Service restart
docker-compose restart ironic-metadata

# View logs
docker-compose logs -f ironic-metadata
```

This comprehensive networking implementation ensures that the ironic-metadata service can be deployed in various environments, from simple development setups to complex production infrastructures with high availability and global routing requirements.

# ironic-metadata

Metadata provider for OpenStack Ironic Standalone - A lightweight service that provides OpenStack-compatible metadata endpoints for bare metal nodes managed by Ironic.

## Features

- **OpenStack Metadata API Compatibility**: Provides standard OpenStack metadata endpoints
- **Multiple API Endpoints**: Supports both OpenStack and EC2-compatible metadata formats
- **Automatic Node Discovery**: Finds nodes based on client IP addresses
- **Configurable**: Environment variable-based configuration
- **Logging**: Structured logging with request tracing

## Supported Endpoints

### OpenStack Format

- `/openstack/latest/meta_data.json` - Node metadata
- `/openstack/latest/network_data.json` - Network configuration
- `/openstack/latest/user_data` - User data (cloud-init)
- `/openstack/latest/vendor_data.json` - Vendor-specific data
- `/openstack/latest/vendor_data2.json` - Extended vendor data

### EC2-Compatible Format

- `/latest/meta-data/` - EC2-style metadata
- `/latest/user-data` - User data

## Configuration

Configure the service using environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `IRONIC_URL` | `http://localhost:6385` | Ironic API endpoint |
| `BIND_ADDR` | `169.254.169.254` | IP address to bind to |
| `BIND_PORT` | `80` | Port to bind to |
| `OS_USERNAME` | _(empty)_ | OpenStack username (optional) |
| `OS_PASSWORD` | _(empty)_ | OpenStack password (optional) |
| `OS_PROJECT_NAME` | _(empty)_ | OpenStack project name (optional) |
| `OS_USER_DOMAIN_NAME` | `default` | OpenStack user domain (optional) |
| `OS_REGION_NAME` | _(empty)_ | OpenStack region (optional) |

## Installation

### From Source

```bash
git clone https://github.com/appkins-org/ironic-metadata.git
cd ironic-metadata
go build -o ironic-metadata ./cmd/ironic-metadata
```

### Running

```bash
# Set environment variables
export IRONIC_URL=http://your-ironic-api:6385
export BIND_ADDR=169.254.169.254
export BIND_PORT=80

# Run the service
./ironic-metadata
```

### Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o ironic-metadata ./cmd/ironic-metadata

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ironic-metadata .
CMD ["./ironic-metadata"]
```

## ConfigDrive Support

The service supports OpenStack ConfigDrive functionality, allowing nodes to use pre-configured metadata, network configuration, and user data.

### ConfigDrive Priority

The service follows this priority order when serving metadata:

1. **ConfigDrive Data**: If a node has `instance_info["configdrive"]` set, it will use that data first
2. **Dynamic Configuration**: Falls back to extracting data from the node's `instance_info` fields

### ConfigDrive Formats

The service supports multiple configdrive formats:

- **JSON String**: Direct JSON configuration in `instance_info["configdrive"]`
- **ISO Image**: ConfigDrive ISO files (parsed using gophercloud utilities)
- **Map Object**: Direct map configuration

### ConfigDrive Structure

Expected configdrive structure:

```json
{
  "meta_data": {
    "hostname": "node-hostname",
    "instance-id": "node-uuid",
    "local-hostname": "node-hostname"
  },
  "user_data": "#cloud-config\npackages:\n  - nginx",
  "network_data": {
    "links": [
      {
        "id": "eth0",
        "type": "physical",
        "mtu": 1500
      }
    ],
    "networks": [
      {
        "id": "network0",
        "type": "ipv4",
        "link": "eth0"
      }
    ]
  },
  "public_keys": {
    "default": "ssh-rsa AAAAB3NzaC1yc2E..."
  }
}
```

### Creating ConfigDrive ISOs

The service can create ConfigDrive ISOs using gophercloud utilities:

```go
// Example: Create a configdrive ISO
userData := "#cloud-config\npackages:\n  - nginx"
metaData := map[string]interface{}{
  "hostname": "test-node",
  "instance-id": "node-uuid",
}
networkData := map[string]interface{}{
  "links": []interface{}{
    map[string]interface{}{
      "id": "eth0", 
      "type": "physical",
      "mtu": 1500,
    },
  },
}

isoBytes, err := createConfigDriveISO(userData, networkData, metaData)
```

## Usage with Ironic

1. **Configure Ironic nodes** with the following instance_info fields:

   ```json
   {
     "user_data": "#cloud-config\n...",
     "public_keys": {
       "default": "ssh-rsa AAAAB3NzaC1yc2E..."
     },
     "network_data": {
       "links": [...],
       "networks": [...]
     }
   }
   ```

2. **Point nodes to the metadata service** by configuring the DHCP server to provide the metadata service IP (169.254.169.254) as a route.

3. **Network Configuration**: Ensure the metadata service can reach the Ironic API and that deploying nodes can reach the metadata service IP.

## How It Works

1. **Client Request**: A deploying node makes an HTTP request to 169.254.169.254
2. **IP Matching**: The service extracts the client IP and searches Ironic for matching nodes
3. **Data Retrieval**: Node information is retrieved from Ironic's API
4. **Response**: Appropriate metadata is returned in the requested format

## API Examples

### Get Metadata

```bash
curl http://169.254.169.254/openstack/latest/meta_data.json
```

Response:

```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "name": "node-01",
  "hostname": "node-01",
  "launch_index": 0,
  "public_keys": {
    "default": "ssh-rsa AAAAB3NzaC1yc2E..."
  },
  "meta": {
    "memory_mb": "8192",
    "cpus": "4"
  }
}
```

### Get Network Data

```bash
curl http://169.254.169.254/openstack/latest/network_data.json
```

Response:

```json
{
  "links": [
    {
      "id": "eth0",
      "type": "physical",
      "mtu": 1500
    }
  ],
  "networks": [
    {
      "id": "network0",
      "type": "ipv4",
      "link": "eth0"
    }
  ],
  "services": []
}
```

### Get User Data

```bash
curl http://169.254.169.254/openstack/latest/user_data
```

## Development

### Prerequisites

- Go 1.24 or later
- Access to an Ironic API

### Building

```bash
go mod download
go build ./...
```

### Testing

```bash
go test ./...
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) for details.

## Troubleshooting

### Common Issues

1. **Node not found**: Ensure the client IP can be matched to a node in Ironic
2. **Connection refused**: Check that Ironic API is accessible and credentials are correct
3. **Empty responses**: Verify that nodes have the required instance_info fields set

### Debug Mode

Enable debug logging by setting the log level:

```bash
export LOG_LEVEL=debug
```

### Network Troubleshooting

Ensure:

- The metadata service can reach Ironic API
- Deploying nodes can reach 169.254.169.254
- Proper routing is configured for metadata IP
- No firewall blocking access to the metadata service

## Advanced Networking and Deployment

### Docker with Macvlan

For local network integration using macvlan networking:

```bash
# Quick setup with auto-detection
./scripts/setup-network.sh
docker-compose up -d ironic-metadata

# Manual setup
docker-compose up -d ironic-metadata
```

See [Docker Deployment Guide](docs/DOCKER_DEPLOYMENT.md) for detailed setup instructions.

### Global Routing

To make 169.254.169.254 globally routable across your network infrastructure, several methods are available:

#### Method 1: Router-Based NAT/DNAT (Recommended)

```bash
# Setup NAT routing automatically
sudo ./scripts/setup-advanced-networking.sh nat-routing

# Or specify host IP manually
sudo ./scripts/setup-advanced-networking.sh nat-routing 192.168.1.100
```

#### Method 2: BGP Anycast Routing

```bash
# Setup BGP routing with BIRD
sudo ./scripts/setup-advanced-networking.sh bgp-bird 1.1.1.1 65001 192.168.1.1 65000
# Arguments: ROUTER_ID LOCAL_AS PEER_IP PEER_AS
```

#### Method 3: High Availability with Keepalived

```bash
# Setup keepalived for HA
sudo ./scripts/setup-advanced-networking.sh keepalived eth0 100 secure_password
# Arguments: INTERFACE PRIORITY AUTH_PASS
```

#### Method 4: Load Balancer with HAProxy

```bash
# Setup HAProxy load balancer
sudo ./scripts/setup-advanced-networking.sh haproxy "192.168.1.10:80,192.168.1.11:80,192.168.1.12:80"
```

### Testing Connectivity

```bash
# Test metadata service connectivity
./scripts/setup-advanced-networking.sh test

# Manual testing
curl http://169.254.169.254/openstack/latest/meta_data.json
ping 169.254.169.254
traceroute 169.254.169.254
```

### Advanced Scenarios

For complex networking scenarios including:

- Multi-site deployments
- Cloud provider integrations (AWS, GCP, Azure)
- Container orchestration (Kubernetes)
- Software-defined networking (OpenStack Neutron, VMware NSX-T)

See the [Advanced Networking Guide](docs/ADVANCED_NETWORKING.md) for comprehensive configuration examples.

### Network Architecture Options

1. **Centralized**: Single metadata service with global routing
2. **Distributed**: Multiple instances with anycast routing
3. **Proxy-based**: Load balancer with multiple backend services
4. **Hybrid**: Combination of methods for different network segments

Each approach has different trade-offs in terms of complexity, availability, and performance. Choose based on your infrastructure requirements.

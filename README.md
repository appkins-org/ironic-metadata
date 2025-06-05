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

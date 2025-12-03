
### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PVE_HOST` | Proxmox host address | `localhost` |
| `PVE_USER` | Proxmox user | `root@pam` |
| `PVE_PASSWORD` | Proxmox password | - |
| `PVE_TOKEN_ID` | API token ID (alternative to password) | - |
| `PVE_TOKEN_SECRET` | API token secret | - |
| `PVE_INSECURE_SKIP_VERIFY` | Skip TLS verification | `true` |
| `LISTEN_ADDRESS` | HTTP server listen address | `:9221` |
| `METRICS_PATH` | Metrics endpoint path | `/metrics` |

### Configuration File

Create a `config.yml` file:

```yaml
proxmox:
  host: "proxmox.example.com"
  port: 8006
  user: "root@pam"
  password: "your-password"
  insecure_skip_verify: true

server:
  listen_address: ":9221"
  metrics_path: "/metrics"


```

Run with: `./pve-exporter -config config.yml`

## Prometheus Configuration

Add to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'proxmox'
    static_configs:
      - targets: ['localhost:9221']
    scrape_interval: 30s
```

## Authentication

### Password Authentication

```bash
PVE_USER=root@pam
PVE_PASSWORD=your-password
```

### API Token Authentication (Recommended)

1. Create an API token in Proxmox:
   - Datacenter → Permissions → API Tokens
   - Create a new token with appropriate permissions

2. Use token credentials:
```bash
PVE_TOKEN_ID=user@realm!tokenid
PVE_TOKEN_SECRET=your-token-secret
```

## Required Proxmox Permissions

For security best practices, create a dedicated monitoring user with **read-only** permissions instead of using the root account.

### Creating a Monitoring User

1. **Create a new user** in Proxmox:
   - Navigate to: Datacenter → Permissions → Users
   - Click "Add" and create user (e.g., `monitoring@pve`)

2. **Assign read-only permissions**:
   - Navigate to: Datacenter → Permissions
   - Click "Add" → "User Permission"
   - Path: `/`
   - User: `monitoring@pve`
   - Role: `PVEAuditor`

3. **Optional: Create API Token** (recommended over password):
   - Navigate to: Datacenter → Permissions → API Tokens
   - Select your monitoring user
   - Click "Add" and create token (e.g., `monitoring@pve!exporter`)
   - **Important**: Uncheck "Privilege Separation" to inherit user permissions
   - Save the token secret securely (shown only once)

### Required Permissions

The exporter requires the following **read-only** permissions:

- **PVEAuditor role** provides:
  - `Sys.Audit` - Read system information (nodes, VMs, containers)
  - `Datastore.Audit` - Read storage information
  - Access to API endpoints for metrics collection

These permissions allow the exporter to:
- ✅ Read node status and metrics
- ✅ Read VM/container status and metrics
- ✅ Read storage information
- ❌ Cannot modify any resources
- ❌ Cannot start/stop VMs or containers
- ❌ Cannot change configurations

## Building from Source

```bash
# Clone the repository
git clone https://github.com/bigtcze/pve-exporter.git
cd pve-exporter

# Build binary
go build -o pve-exporter .

# Build Docker image
docker build -t pve-exporter .
```

## Development

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Run locally
go run . -config config.example.yml
```

## Grafana Dashboard

Example queries for Grafana:

**Node CPU Load**:
```promql
pve_node_cpu_load{node="proxmox"}
```

**VM Memory Usage**:
```promql
pve_vm_memory_used_bytes / pve_vm_memory_max_bytes * 100
```



## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by the Proxmox InfluxDB integration
- Built with [Prometheus Go client](https://github.com/prometheus/client_golang)

## Support

For issues and questions, please use the [GitHub issue tracker](https://github.com/bigtcze/pve-exporter/issues).

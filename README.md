# Proxmox Exporter for Prometheus

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/bigtcze/pve-exporter)](https://goreportcard.com/report/github.com/bigtcze/pve-exporter)

A Prometheus exporter for Proxmox Virtual Environment (PVE) that collects the same metrics Proxmox sends to InfluxDB, including comprehensive ZFS statistics and ARC metrics.

## Features

- **Proxmox Metrics**: Collects node, VM, container, and storage metrics via Proxmox API
- **ZFS Support**: Complete ZFS metrics including pool statistics and ARC (Adaptive Replacement Cache) data
- **Flexible Authentication**: Supports both password and API token authentication
- **Docker Ready**: Multi-arch Docker images available
- **Lightweight**: Minimal resource footprint with efficient metric collection
- **Secure**: Runs as non-root user in Docker, supports TLS verification

## Metrics Collected

### Proxmox Metrics

- **Node Metrics**: Status, uptime, CPU load, memory usage, swap usage
- **VM/Container Metrics**: Status, uptime, CPU usage, memory, disk I/O, network I/O
- **Storage Metrics**: Total, used, and available space per storage

### ZFS Metrics

- **ARC Statistics**: Size, hit/miss ratios, c_max, c_min, target size, MRU/MFU sizes
- **Pool Metrics**: Health, capacity, allocation, fragmentation
- **Memory Throttle Events**: Track ARC memory pressure

## Quick Start

### Docker (Recommended)

```bash
docker run -d \
  --name pve-exporter \
  -p 9221:9221 \
  -e PVE_HOST=proxmox.example.com \
  -e PVE_USER=root@pam \
  -e PVE_PASSWORD=your-password \
  -e PVE_INSECURE_SKIP_VERIFY=true \
  -v /proc/spl/kstat/zfs:/proc/spl/kstat/zfs:ro \
  ghcr.io/bigtcze/pve-exporter:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  pve-exporter:
    image: ghcr.io/bigtcze/pve-exporter:latest
    container_name: pve-exporter
    restart: unless-stopped
    ports:
      - "9221:9221"
    environment:
      PVE_HOST: "proxmox.example.com"
      PVE_USER: "root@pam"
      PVE_PASSWORD: "your-password"
      PVE_INSECURE_SKIP_VERIFY: "true"
      ZFS_ENABLED: "true"
    volumes:
      - /proc/spl/kstat/zfs:/proc/spl/kstat/zfs:ro
```

### Binary

Download the latest release from the [releases page](https://github.com/bigtcze/pve-exporter/releases):

```bash
./pve-exporter -config config.yml
```

## Configuration

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
| `ZFS_ENABLED` | Enable ZFS metrics collection | `true` |
| `ZFS_KSTAT_PATH` | Path to ZFS kstat directory | `/proc/spl/kstat/zfs` |

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

zfs:
  enabled: true
  kstat_path: "/proc/spl/kstat/zfs"
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

**ZFS ARC Hit Rate**:
```promql
rate(zfs_arc_hits_total[5m]) / (rate(zfs_arc_hits_total[5m]) + rate(zfs_arc_misses_total[5m])) * 100
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

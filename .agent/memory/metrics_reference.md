# Metrics Reference

Complete reference of all metrics exported by the Proxmox Exporter.

## Proxmox Node Metrics

### pve_node_up
- **Type**: Gauge
- **Description**: Node is up and reachable (1=up, 0=down)
- **Labels**: `node`

### pve_node_uptime_seconds
- **Type**: Gauge
- **Description**: Node uptime in seconds
- **Labels**: `node`

### pve_node_cpu_load
- **Type**: Gauge
- **Description**: Node CPU load (0.0 to 1.0+)
- **Labels**: `node`

### pve_node_memory_total_bytes
- **Type**: Gauge
- **Description**: Total memory in bytes
- **Labels**: `node`

### pve_node_memory_used_bytes
- **Type**: Gauge
- **Description**: Used memory in bytes
- **Labels**: `node`

### pve_node_memory_free_bytes
- **Type**: Gauge
- **Description**: Free memory in bytes
- **Labels**: `node`

### pve_node_swap_total_bytes
- **Type**: Gauge
- **Description**: Total swap in bytes
- **Labels**: `node`

### pve_node_swap_used_bytes
- **Type**: Gauge
- **Description**: Used swap in bytes
- **Labels**: `node`

### pve_node_swap_free_bytes
- **Type**: Gauge
- **Description**: Free swap in bytes
- **Labels**: `node`

## VM/Container Metrics

### pve_vm_status
- **Type**: Gauge
- **Description**: VM/Container status (1=running, 0=stopped)
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_uptime_seconds
- **Type**: Gauge
- **Description**: VM/Container uptime in seconds
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_cpu_usage
- **Type**: Gauge
- **Description**: VM/Container CPU usage (0.0 to number of CPUs)
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_cpus
- **Type**: Gauge
- **Description**: Number of CPUs allocated
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_memory_used_bytes
- **Type**: Gauge
- **Description**: Memory usage in bytes
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_memory_max_bytes
- **Type**: Gauge
- **Description**: Maximum memory in bytes
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_disk_used_bytes
- **Type**: Gauge
- **Description**: Disk usage in bytes
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_disk_max_bytes
- **Type**: Gauge
- **Description**: Maximum disk in bytes
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_network_in_bytes_total
- **Type**: Counter
- **Description**: Network input in bytes (cumulative)
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_network_out_bytes_total
- **Type**: Counter
- **Description**: Network output in bytes (cumulative)
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_disk_read_bytes_total
- **Type**: Counter
- **Description**: Disk read in bytes (cumulative)
- **Labels**: `node`, `type`, `vmid`, `name`

### pve_vm_disk_write_bytes_total
- **Type**: Counter
- **Description**: Disk write in bytes (cumulative)
- **Labels**: `node`, `type`, `vmid`, `name`

## Storage Metrics

### pve_storage_total_bytes
- **Type**: Gauge
- **Description**: Total storage size in bytes
- **Labels**: `node`, `storage`, `type`

### pve_storage_used_bytes
- **Type**: Gauge
- **Description**: Used storage in bytes
- **Labels**: `node`, `storage`, `type`

### pve_storage_available_bytes
- **Type**: Gauge
- **Description**: Available storage in bytes
- **Labels**: `node`, `storage`, `type`

## ZFS ARC Metrics

### zfs_arc_size_bytes
- **Type**: Gauge
- **Description**: Current size of ARC in bytes

### zfs_arc_c_max_bytes
- **Type**: Gauge
- **Description**: Maximum size of ARC in bytes

### zfs_arc_c_min_bytes
- **Type**: Gauge
- **Description**: Minimum size of ARC in bytes

### zfs_arc_c_bytes
- **Type**: Gauge
- **Description**: Target size of ARC in bytes

### zfs_arc_p_bytes
- **Type**: Gauge
- **Description**: Target size of MRU in bytes

### zfs_arc_hits_total
- **Type**: Counter
- **Description**: Total number of ARC hits

### zfs_arc_misses_total
- **Type**: Counter
- **Description**: Total number of ARC misses

### zfs_arc_target_size_bytes
- **Type**: Gauge
- **Description**: Target size of ARC

### zfs_arc_mru_size_bytes
- **Type**: Gauge
- **Description**: Size of MRU (Most Recently Used) list in bytes

### zfs_arc_mfu_size_bytes
- **Type**: Gauge
- **Description**: Size of MFU (Most Frequently Used) list in bytes

### zfs_arc_mru_ghost_size_bytes
- **Type**: Gauge
- **Description**: Size of MRU ghost list in bytes

### zfs_arc_mfu_ghost_size_bytes
- **Type**: Gauge
- **Description**: Size of MFU ghost list in bytes

### zfs_arc_data_size_bytes
- **Type**: Gauge
- **Description**: Size of data in ARC in bytes

### zfs_arc_metadata_size_bytes
- **Type**: Gauge
- **Description**: Size of metadata in ARC in bytes

### zfs_arc_evict_skip_total
- **Type**: Counter
- **Description**: Total number of evictions skipped

### zfs_arc_memory_throttle_count_total
- **Type**: Counter
- **Description**: Total number of memory throttle events

## ZFS Pool Metrics

### zfs_pool_health
- **Type**: Gauge
- **Description**: Health status of ZFS pool (0=degraded, 1=online)
- **Labels**: `pool`

### zfs_pool_capacity_bytes
- **Type**: Gauge
- **Description**: Total capacity of ZFS pool in bytes
- **Labels**: `pool`

### zfs_pool_allocated_bytes
- **Type**: Gauge
- **Description**: Allocated space in ZFS pool in bytes
- **Labels**: `pool`

### zfs_pool_free_bytes
- **Type**: Gauge
- **Description**: Free space in ZFS pool in bytes
- **Labels**: `pool`

### zfs_pool_fragmentation_percent
- **Type**: Gauge
- **Description**: Fragmentation percentage of ZFS pool
- **Labels**: `pool`

## Example Queries

### Calculate ARC Hit Rate
```promql
rate(zfs_arc_hits_total[5m]) / (rate(zfs_arc_hits_total[5m]) + rate(zfs_arc_misses_total[5m])) * 100
```

### VM Memory Usage Percentage
```promql
pve_vm_memory_used_bytes / pve_vm_memory_max_bytes * 100
```

### Storage Usage Percentage
```promql
pve_storage_used_bytes / pve_storage_total_bytes * 100
```

### Network Traffic Rate
```promql
rate(pve_vm_network_in_bytes_total[5m])
```

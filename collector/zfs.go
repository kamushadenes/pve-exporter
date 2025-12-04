package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// collectZFSMetrics collects ZFS pool and ARC metrics
func (c *ProxmoxCollector) collectZFSMetrics(ch chan<- prometheus.Metric) {
	c.collectZFSPoolMetrics(ch)
	c.collectZFSARCMetrics(ch)
}

// collectZFSPoolMetrics collects ZFS pool metrics from Proxmox API
func (c *ProxmoxCollector) collectZFSPoolMetrics(ch chan<- prometheus.Metric) {
	// Get list of nodes
	nodesData, err := c.apiRequest("/nodes")
	if err != nil {
		log.Printf("Error fetching nodes for ZFS metrics: %v", err)
		return
	}

	var nodesResult struct {
		Data []struct {
			Node string `json:"node"`
		} `json:"data"`
	}

	if err := json.Unmarshal(nodesData, &nodesResult); err != nil {
		log.Printf("Error unmarshaling nodes for ZFS metrics: %v", err)
		return
	}

	for _, node := range nodesResult.Data {
		path := fmt.Sprintf("/nodes/%s/disks/zfs", node.Node)
		data, err := c.apiRequest(path)
		if err != nil {
			// ZFS might not be installed or configured on this node
			continue
		}

		var result struct {
			Data []struct {
				Name   string  `json:"name"`
				Health string  `json:"health"`
				Size   float64 `json:"size"`
				Alloc  float64 `json:"alloc"`
				Free   float64 `json:"free"`
				Frag   float64 `json:"frag"`
			} `json:"data"`
		}

		if err := json.Unmarshal(data, &result); err != nil {
			log.Printf("Error unmarshaling ZFS pools for node %s: %v", node.Node, err)
			continue
		}

		for _, pool := range result.Data {
			health := 0.0
			if pool.Health == "ONLINE" {
				health = 1.0
			}

			ch <- prometheus.MustNewConstMetric(c.zfsPoolHealth, prometheus.GaugeValue, health, node.Node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolSize, prometheus.GaugeValue, pool.Size, node.Node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolAlloc, prometheus.GaugeValue, pool.Alloc, node.Node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolFree, prometheus.GaugeValue, pool.Free, node.Node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolFrag, prometheus.GaugeValue, pool.Frag, node.Node, pool.Name)
		}
	}
}

// collectZFSARCMetrics collects ZFS ARC metrics from /proc/spl/kstat/zfs/arcstats
func (c *ProxmoxCollector) collectZFSARCMetrics(ch chan<- prometheus.Metric) {
	// We assume the exporter runs on the Proxmox host, so we read the file directly.
	// If running in a container/VM without access, this will fail gracefully.
	file, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		// Silent return if file doesn't exist (e.g. not ZFS, or remote)
		return
	}
	defer file.Close()

	// Since we are running on the host, we associate these metrics with the local node name if possible.
	// However, discovering the local node name from within the collector is tricky without API.
	// For simplicity, we'll try to use the first node found in config or API, or just "localhost" if all else fails.
	// Better approach: Since these are host-level metrics, we should ideally label them with the node name.
	// We can reuse the node discovery logic, but that's expensive.
	// Let's try to get the hostname from os.Hostname()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	scanner := bufio.NewScanner(file)
	// Skip header lines (usually first 2)
	// name    type    data
	// hits    4    12345
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		valueStr := fields[2]
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		switch name {
		case "size":
			ch <- prometheus.MustNewConstMetric(c.zfsARCSize, prometheus.GaugeValue, value, hostname)
		case "c_min":
			ch <- prometheus.MustNewConstMetric(c.zfsARCMinSize, prometheus.GaugeValue, value, hostname)
		case "c_max":
			ch <- prometheus.MustNewConstMetric(c.zfsARCMaxSize, prometheus.GaugeValue, value, hostname)
		case "hits":
			ch <- prometheus.MustNewConstMetric(c.zfsARCHits, prometheus.CounterValue, value, hostname)
		case "misses":
			ch <- prometheus.MustNewConstMetric(c.zfsARCMisses, prometheus.CounterValue, value, hostname)
		case "c":
			ch <- prometheus.MustNewConstMetric(c.zfsARCTargetSize, prometheus.GaugeValue, value, hostname)
		case "l2_hits":
			ch <- prometheus.MustNewConstMetric(c.zfsARCL2Hits, prometheus.CounterValue, value, hostname)
		case "l2_misses":
			ch <- prometheus.MustNewConstMetric(c.zfsARCL2Misses, prometheus.CounterValue, value, hostname)
		case "l2_size":
			ch <- prometheus.MustNewConstMetric(c.zfsARCL2Size, prometheus.GaugeValue, value, hostname)
		case "l2_hdr_size":
			ch <- prometheus.MustNewConstMetric(c.zfsARCL2HeaderSize, prometheus.GaugeValue, value, hostname)
		}
	}

	// Calculate hit ratio if possible (requires state, but we are stateless here).
	// Prometheus usually calculates rates on server side.
	// However, we can provide a point-in-time ratio if we want, but it's better to let Prometheus do `rate(hits) / (rate(hits) + rate(misses))`
	// We defined zfsARCHitRatio, let's see if we can populate it.
	// Without keeping state of previous scrape, we can only provide the ratio of TOTAL hits/misses since boot, which is not very useful.
	// So we might skip zfsARCHitRatio and let users calculate it.
	// OR, we can just export the raw counters (which we did).
	// I'll leave zfsARCHitRatio unpopulated for now as it's better calculated via PromQL.
}

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

// collectZFSMetricsWithNodes collects ZFS metrics using pre-fetched nodes list
func (c *ProxmoxCollector) collectZFSMetricsWithNodes(ch chan<- prometheus.Metric, nodes []string) {
	c.collectZFSPoolMetricsWithNodes(ch, nodes)
	c.collectZFSARCMetrics(ch)
}

// collectZFSPoolMetricsWithNodes collects ZFS pool metrics using pre-fetched nodes list
func (c *ProxmoxCollector) collectZFSPoolMetricsWithNodes(ch chan<- prometheus.Metric, nodes []string) {
	for _, node := range nodes {
		path := fmt.Sprintf("/nodes/%s/disks/zfs", node)
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
			log.Printf("Error unmarshaling ZFS pools for node %s: %v", node, err)
			continue
		}

		for _, pool := range result.Data {
			health := 0.0
			if pool.Health == "ONLINE" {
				health = 1.0
			}

			ch <- prometheus.MustNewConstMetric(c.zfsPoolHealth, prometheus.GaugeValue, health, node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolSize, prometheus.GaugeValue, pool.Size, node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolAlloc, prometheus.GaugeValue, pool.Alloc, node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolFree, prometheus.GaugeValue, pool.Free, node, pool.Name)
			ch <- prometheus.MustNewConstMetric(c.zfsPoolFrag, prometheus.GaugeValue, pool.Frag, node, pool.Name)
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

	// Collect all values first to calculate hit ratio
	var hits, misses float64

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
			hits = value
			ch <- prometheus.MustNewConstMetric(c.zfsARCHits, prometheus.CounterValue, value, hostname)
		case "misses":
			misses = value
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

	// Calculate and emit hit ratio percent (hits / (hits + misses) * 100)
	// This is the cumulative hit ratio since boot, useful for overall ARC efficiency
	total := hits + misses
	if total > 0 {
		hitRatioPercent := (hits / total) * 100
		ch <- prometheus.MustNewConstMetric(c.zfsARCHitRatio, prometheus.GaugeValue, hitRatioPercent, hostname)
	}
}

package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// collectZFSMetricsWithNodes collects ZFS metrics using pre-fetched nodes list
func (c *ProxmoxCollector) collectZFSMetricsWithNodes(ch chan<- prometheus.Metric, nodes []string) {
	c.collectZFSPoolMetricsWithNodes(ch, nodes)
	c.collectZFSARCMetrics(ch)
}

// collectZFSPoolMetricsWithNodes collects ZFS pool metrics using pre-fetched nodes list
func (c *ProxmoxCollector) collectZFSPoolMetricsWithNodes(ch chan<- prometheus.Metric, nodes []string) {
	// Process nodes in parallel for better performance
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()

			path := fmt.Sprintf("/nodes/%s/disks/zfs", nodeName)
			data, err := c.apiRequest(path)
			if err != nil {
				// ZFS might not be installed or configured on this node
				return
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
				log.Printf("Error unmarshaling ZFS pools for node %s: %v", nodeName, err)
				return
			}

			for _, pool := range result.Data {
				health := 0.0
				if pool.Health == "ONLINE" {
					health = 1.0
				}

				ch <- prometheus.MustNewConstMetric(c.zfsPoolHealth, prometheus.GaugeValue, health, nodeName, pool.Name)
				ch <- prometheus.MustNewConstMetric(c.zfsPoolSize, prometheus.GaugeValue, pool.Size, nodeName, pool.Name)
				ch <- prometheus.MustNewConstMetric(c.zfsPoolAlloc, prometheus.GaugeValue, pool.Alloc, nodeName, pool.Name)
				ch <- prometheus.MustNewConstMetric(c.zfsPoolFree, prometheus.GaugeValue, pool.Free, nodeName, pool.Name)
				ch <- prometheus.MustNewConstMetric(c.zfsPoolFrag, prometheus.GaugeValue, pool.Frag, nodeName, pool.Name)
			}
		}(node)
	}
	wg.Wait()
}

// arcMetricHandler defines how to handle a specific ARC metric
type arcMetricHandler struct {
	metric      *prometheus.Desc
	valueType   prometheus.ValueType
	trackHits   bool // if true, also store value in hits variable
	trackMisses bool // if true, also store value in misses variable
}

// collectZFSARCMetrics collects ZFS ARC metrics from /proc/spl/kstat/zfs/arcstats
func (c *ProxmoxCollector) collectZFSARCMetrics(ch chan<- prometheus.Metric) {
	file, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		return
	}
	defer file.Close()

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// Map metric names to their handlers
	handlers := map[string]arcMetricHandler{
		"size":        {c.zfsARCSize, prometheus.GaugeValue, false, false},
		"c_min":       {c.zfsARCMinSize, prometheus.GaugeValue, false, false},
		"c_max":       {c.zfsARCMaxSize, prometheus.GaugeValue, false, false},
		"hits":        {c.zfsARCHits, prometheus.CounterValue, true, false},
		"misses":      {c.zfsARCMisses, prometheus.CounterValue, false, true},
		"c":           {c.zfsARCTargetSize, prometheus.GaugeValue, false, false},
		"l2_hits":     {c.zfsARCL2Hits, prometheus.CounterValue, false, false},
		"l2_misses":   {c.zfsARCL2Misses, prometheus.CounterValue, false, false},
		"l2_size":     {c.zfsARCL2Size, prometheus.GaugeValue, false, false},
		"l2_hdr_size": {c.zfsARCL2HeaderSize, prometheus.GaugeValue, false, false},
	}

	var hits, misses float64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		value, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			continue
		}

		if handler, ok := handlers[name]; ok {
			ch <- prometheus.MustNewConstMetric(handler.metric, handler.valueType, value, hostname)
			if handler.trackHits {
				hits = value
			}
			if handler.trackMisses {
				misses = value
			}
		}
	}

	// Calculate and emit hit ratio percent
	total := hits + misses
	if total > 0 {
		hitRatioPercent := (hits / total) * 100
		ch <- prometheus.MustNewConstMetric(c.zfsARCHitRatio, prometheus.GaugeValue, hitRatioPercent, hostname)
	}
}

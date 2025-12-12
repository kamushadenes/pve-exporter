package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// collectNodeMetricsWithNodes collects node-level metrics from pre-fetched data
func (c *ProxmoxCollector) collectNodeMetricsWithNodes(ch chan<- prometheus.Metric, data []byte) {
	var result struct {
		Data []struct {
			Node    string  `json:"node"`
			Status  string  `json:"status"`
			Uptime  float64 `json:"uptime"`
			CPU     float64 `json:"cpu"`
			MaxCPU  float64 `json:"maxcpu"`
			Mem     float64 `json:"mem"`
			MaxMem  float64 `json:"maxmem"`
			Disk    float64 `json:"disk"`
			MaxDisk float64 `json:"maxdisk"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling nodes data: %v", err)
		return
	}

	// Collect basic metrics first, then fetch detailed metrics in parallel
	var wg sync.WaitGroup
	for _, node := range result.Data {
		up := 0.0
		if node.Status == "online" {
			up = 1.0
		}

		ch <- prometheus.MustNewConstMetric(c.nodeUp, prometheus.GaugeValue, up, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeUptime, prometheus.GaugeValue, node.Uptime, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeCPULoad, prometheus.GaugeValue, node.CPU, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeCPUs, prometheus.GaugeValue, node.MaxCPU, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeMemoryTotal, prometheus.GaugeValue, node.MaxMem, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeMemoryUsed, prometheus.GaugeValue, node.Mem, node.Node)
		ch <- prometheus.MustNewConstMetric(c.nodeMemoryFree, prometheus.GaugeValue, node.MaxMem-node.Mem, node.Node)

		// Fetch detailed node status for additional metrics in parallel
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			c.collectNodeDetailedMetrics(ch, nodeName)
		}(node.Node)
	}
	wg.Wait()
}

// collectNodeDetailedMetrics fetches detailed node status from /nodes/{node}/status
func (c *ProxmoxCollector) collectNodeDetailedMetrics(ch chan<- prometheus.Metric, nodeName string) {
	path := fmt.Sprintf("/nodes/%s/status", nodeName)
	data, err := c.apiRequest(path)
	if err != nil {
		log.Printf("Error fetching node status for %s: %v", nodeName, err)
		return
	}

	var result struct {
		Data struct {
			LoadAvg []string `json:"loadavg"`
			Wait    float64  `json:"wait"`
			Idle    float64  `json:"idle"`
			KSM     struct {
				Shared float64 `json:"shared"`
			} `json:"ksm"`
			CPUInfo struct {
				Cores   float64 `json:"cores"`
				Sockets float64 `json:"sockets"`
				Mhz     string  `json:"mhz"`
			} `json:"cpuinfo"`
			Rootfs struct {
				Total float64 `json:"total"`
				Used  float64 `json:"used"`
				Free  float64 `json:"free"`
			} `json:"rootfs"`
			Swap struct {
				Total float64 `json:"total"`
				Used  float64 `json:"used"`
				Free  float64 `json:"free"`
			} `json:"swap"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling node status for %s: %v", nodeName, err)
		return
	}

	// Load averages
	if len(result.Data.LoadAvg) >= 3 {
		if load1, err := strconv.ParseFloat(result.Data.LoadAvg[0], 64); err == nil {
			ch <- prometheus.MustNewConstMetric(c.nodeLoad1, prometheus.GaugeValue, load1, nodeName)
		}
		if load5, err := strconv.ParseFloat(result.Data.LoadAvg[1], 64); err == nil {
			ch <- prometheus.MustNewConstMetric(c.nodeLoad5, prometheus.GaugeValue, load5, nodeName)
		}
		if load15, err := strconv.ParseFloat(result.Data.LoadAvg[2], 64); err == nil {
			ch <- prometheus.MustNewConstMetric(c.nodeLoad15, prometheus.GaugeValue, load15, nodeName)
		}
	}

	// I/O wait and idle
	ch <- prometheus.MustNewConstMetric(c.nodeIOWait, prometheus.GaugeValue, result.Data.Wait, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeIdle, prometheus.GaugeValue, result.Data.Idle, nodeName)

	// CPU frequency
	if mhz, err := strconv.ParseFloat(result.Data.CPUInfo.Mhz, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.nodeCPUMhz, prometheus.GaugeValue, mhz, nodeName)
	}

	// Root filesystem
	ch <- prometheus.MustNewConstMetric(c.nodeRootfsTotal, prometheus.GaugeValue, result.Data.Rootfs.Total, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeRootfsUsed, prometheus.GaugeValue, result.Data.Rootfs.Used, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeRootfsFree, prometheus.GaugeValue, result.Data.Rootfs.Free, nodeName)

	// CPU topology
	ch <- prometheus.MustNewConstMetric(c.nodeCPUCores, prometheus.GaugeValue, result.Data.CPUInfo.Cores, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeCPUSockets, prometheus.GaugeValue, result.Data.CPUInfo.Sockets, nodeName)

	// KSM shared memory
	ch <- prometheus.MustNewConstMetric(c.nodeKSMShared, prometheus.GaugeValue, result.Data.KSM.Shared, nodeName)

	// Swap (from detailed status)
	ch <- prometheus.MustNewConstMetric(c.nodeSwapTotal, prometheus.GaugeValue, result.Data.Swap.Total, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeSwapUsed, prometheus.GaugeValue, result.Data.Swap.Used, nodeName)
	ch <- prometheus.MustNewConstMetric(c.nodeSwapFree, prometheus.GaugeValue, result.Data.Swap.Free, nodeName)
}

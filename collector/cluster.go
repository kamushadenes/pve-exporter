package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// collectClusterMetrics collects cluster status and HA resource metrics
func (c *ProxmoxCollector) collectClusterMetrics(ch chan<- prometheus.Metric) {
	// Fetch cluster status
	data, err := c.apiRequest("/cluster/status")
	if err != nil {
		log.Printf("Error fetching cluster status: %v", err)
		return
	}

	var result struct {
		Data []struct {
			Type    string `json:"type"` // "cluster" or "node"
			Name    string `json:"name"`
			Quorate int    `json:"quorate"` // 1 if cluster has quorum
			Online  int    `json:"online"`  // 1 if node is online
			Nodes   int    `json:"nodes"`   // number of nodes (only in cluster type)
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling cluster status: %v", err)
		return
	}

	var nodesTotal, nodesOnline int
	var hasClusterEntry bool
	for _, item := range result.Data {
		if item.Type == "cluster" {
			// Cluster-level info
			ch <- prometheus.MustNewConstMetric(c.clusterQuorate, prometheus.GaugeValue, float64(item.Quorate))
			nodesTotal = item.Nodes
			hasClusterEntry = true
		} else if item.Type == "node" {
			// Count nodes manually (for single-node or when Nodes field is 0)
			if nodesTotal == 0 {
				nodesTotal++
			}
			// Count online nodes
			if item.Online == 1 {
				nodesOnline++
			}
		}
	}

	// Single node is always quorate
	if !hasClusterEntry {
		ch <- prometheus.MustNewConstMetric(c.clusterQuorate, prometheus.GaugeValue, 1)
	}

	ch <- prometheus.MustNewConstMetric(c.clusterNodesTotal, prometheus.GaugeValue, float64(nodesTotal))
	ch <- prometheus.MustNewConstMetric(c.clusterNodesOnline, prometheus.GaugeValue, float64(nodesOnline))

	// Fetch HA resources
	haData, err := c.apiRequest("/cluster/ha/resources")
	if err != nil {
		// HA might not be configured, silently skip
		ch <- prometheus.MustNewConstMetric(c.haResourcesTotal, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.haResourcesActive, prometheus.GaugeValue, 0)
		return
	}

	var haResult struct {
		Data []struct {
			Sid   string `json:"sid"`   // Resource ID
			State string `json:"state"` // started, stopped, etc.
		} `json:"data"`
	}

	if err := json.Unmarshal(haData, &haResult); err != nil {
		log.Printf("Error unmarshaling HA resources: %v", err)
		return
	}

	var haTotal, haActive int
	for _, res := range haResult.Data {
		haTotal++
		if res.State == "started" {
			haActive++
		}
	}

	ch <- prometheus.MustNewConstMetric(c.haResourcesTotal, prometheus.GaugeValue, float64(haTotal))
	ch <- prometheus.MustNewConstMetric(c.haResourcesActive, prometheus.GaugeValue, float64(haActive))
}

// collectReplicationMetrics collects replication job status metrics
func (c *ProxmoxCollector) collectReplicationMetrics(ch chan<- prometheus.Metric) {
	data, err := c.apiRequest("/cluster/replication")
	if err != nil {
		// Replication might not be configured, silently skip
		return
	}

	var result struct {
		Data []struct {
			ID        string  `json:"id"`        // Job ID (e.g., "100-0")
			Guest     int64   `json:"guest"`     // VM/LXC ID
			JobNum    int     `json:"jobnum"`    // Job number
			LastSync  int64   `json:"last_sync"` // Last sync timestamp
			Duration  float64 `json:"duration"`  // Duration in seconds
			FailCount int     `json:"fail_count"`
			Error     string  `json:"error"` // Error message if any
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling replication data: %v", err)
		return
	}

	for _, job := range result.Data {
		guest := strconv.FormatInt(job.Guest, 10)
		jobID := job.ID

		// Last sync timestamp
		if job.LastSync > 0 {
			ch <- prometheus.MustNewConstMetric(c.replicationLastSync, prometheus.GaugeValue, float64(job.LastSync), guest, jobID)
		}

		// Duration
		ch <- prometheus.MustNewConstMetric(c.replicationDuration, prometheus.GaugeValue, job.Duration, guest, jobID)

		// Status (1=OK, 0=error)
		status := 1.0
		if job.FailCount > 0 || job.Error != "" {
			status = 0
		}
		ch <- prometheus.MustNewConstMetric(c.replicationStatus, prometheus.GaugeValue, status, guest, jobID)
	}
}

// collectCertificateMetrics collects SSL certificate expiry metrics
func (c *ProxmoxCollector) collectCertificateMetrics(ch chan<- prometheus.Metric, nodes []string) {
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()

			data, err := c.apiRequest(fmt.Sprintf("/nodes/%s/certificates/info", nodeName))
			if err != nil {
				log.Printf("Error fetching certificates for node %s: %v", nodeName, err)
				return
			}

			var result struct {
				Data []struct {
					Filename string `json:"filename"`
					NotAfter int64  `json:"notafter"` // Unix timestamp
				} `json:"data"`
			}

			if err := json.Unmarshal(data, &result); err != nil {
				log.Printf("Error unmarshaling certificates for node %s: %v", nodeName, err)
				return
			}

			// Find the main pveproxy certificate
			now := time.Now().Unix()
			for _, cert := range result.Data {
				if cert.Filename == "pveproxy-ssl.pem" || cert.Filename == "pve-ssl.pem" {
					expirySeconds := float64(cert.NotAfter - now)
					ch <- prometheus.MustNewConstMetric(c.certificateExpiry, prometheus.GaugeValue, expirySeconds, nodeName)
					return
				}
			}

			// If no specific cert found, use the first one
			if len(result.Data) > 0 {
				expirySeconds := float64(result.Data[0].NotAfter - now)
				ch <- prometheus.MustNewConstMetric(c.certificateExpiry, prometheus.GaugeValue, expirySeconds, nodeName)
			}
		}(node)
	}
	wg.Wait()
}

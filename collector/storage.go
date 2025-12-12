package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// collectStorageMetrics collects storage metrics for all nodes in parallel
func (c *ProxmoxCollector) collectStorageMetrics(ch chan<- prometheus.Metric, nodes []string) {
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()

			path := fmt.Sprintf("/nodes/%s/storage", nodeName)
			storageData, err := c.apiRequest(path)
			if err != nil {
				log.Printf("Error fetching storage for node %s: %v", nodeName, err)
				return
			}

			var result struct {
				Data []struct {
					Storage      string  `json:"storage"`
					Type         string  `json:"type"`
					Total        float64 `json:"total"`
					Used         float64 `json:"used"`
					Avail        float64 `json:"avail"`
					Active       int     `json:"active"`
					Enabled      int     `json:"enabled"`
					Shared       int     `json:"shared"`
					UsedFraction float64 `json:"used_fraction"`
				} `json:"data"`
			}

			if err := json.Unmarshal(storageData, &result); err != nil {
				log.Printf("Error unmarshaling storage for node %s: %v", nodeName, err)
				return
			}

			for _, storage := range result.Data {
				labels := []string{nodeName, storage.Storage, storage.Type}
				ch <- prometheus.MustNewConstMetric(c.storageTotal, prometheus.GaugeValue, storage.Total, labels...)
				ch <- prometheus.MustNewConstMetric(c.storageUsed, prometheus.GaugeValue, storage.Used, labels...)
				ch <- prometheus.MustNewConstMetric(c.storageAvail, prometheus.GaugeValue, storage.Avail, labels...)
				ch <- prometheus.MustNewConstMetric(c.storageActive, prometheus.GaugeValue, float64(storage.Active), labels...)
				ch <- prometheus.MustNewConstMetric(c.storageEnabled, prometheus.GaugeValue, float64(storage.Enabled), labels...)
				ch <- prometheus.MustNewConstMetric(c.storageShared, prometheus.GaugeValue, float64(storage.Shared), labels...)
				ch <- prometheus.MustNewConstMetric(c.storageUsedFraction, prometheus.GaugeValue, storage.UsedFraction, labels...)
			}
		}(node)
	}
	wg.Wait()
}

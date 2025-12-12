package collector

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Collect implements prometheus.Collector
func (c *ProxmoxCollector) Collect(ch chan<- prometheus.Metric) {
	// Authenticate if needed
	if err := c.authenticate(); err != nil {
		log.Printf("Error during authentication: %v", err)
		return
	}

	// Fetch nodes list ONCE and reuse across all collection functions
	nodesData, err := c.apiRequest("/nodes")
	if err != nil {
		log.Printf("Error fetching nodes: %v", err)
		return
	}

	var nodesResult struct {
		Data []struct {
			Node string `json:"node"`
		} `json:"data"`
	}

	if err := json.Unmarshal(nodesData, &nodesResult); err != nil {
		log.Printf("Error unmarshaling nodes: %v", err)
		return
	}

	// Extract node names
	nodes := make([]string, len(nodesResult.Data))
	for i, n := range nodesResult.Data {
		nodes[i] = n.Node
	}

	// OPTIMIZATION #6: Fetch all guests ONCE using /cluster/resources (single API call)
	// This replaces N×2 per-node calls (/qemu + /lxc per node)
	guests := make(map[string]GuestInfo)
	resourcesData, err := c.apiRequest("/cluster/resources?type=vm")
	if err == nil {
		var resourcesResult struct {
			Data []struct {
				VMID   int64  `json:"vmid"`
				Node   string `json:"node"`
				Name   string `json:"name"`
				Type   string `json:"type"` // "qemu" or "lxc"
				Status string `json:"status"`
			} `json:"data"`
		}
		if json.Unmarshal(resourcesData, &resourcesResult) == nil {
			for _, res := range resourcesResult.Data {
				vmid := strconv.FormatInt(res.VMID, 10)
				guests[vmid] = GuestInfo{Node: res.Node, Name: res.Name, Type: res.Type}
			}
		}
	}

	// Run all collection functions in parallel for better performance
	var wg sync.WaitGroup

	wg.Add(10)

	go func() {
		defer wg.Done()
		c.collectNodeMetricsWithNodes(ch, nodesData)
	}()

	go func() {
		defer wg.Done()
		c.collectVMMetricsWithNodes(ch, nodes)
	}()

	go func() {
		defer wg.Done()
		c.collectStorageMetrics(ch, nodes)
	}()

	go func() {
		defer wg.Done()
		c.collectZFSMetricsWithNodes(ch, nodes)
	}()

	go func() {
		defer wg.Done()
		c.collectSensorsMetrics(ch)
	}()

	go func() {
		defer wg.Done()
		c.collectDiskMetrics(ch)
	}()

	go func() {
		defer wg.Done()
		// OPTIMIZATION #2: Pass pre-fetched guest data to avoid duplicate API calls
		c.collectBackupMetricsWithGuests(ch, nodes, guests)
	}()

	go func() {
		defer wg.Done()
		c.collectClusterMetrics(ch)
	}()

	go func() {
		defer wg.Done()
		c.collectReplicationMetrics(ch)
	}()

	go func() {
		defer wg.Done()
		c.collectCertificateMetrics(ch, nodes)
	}()

	wg.Wait()
}

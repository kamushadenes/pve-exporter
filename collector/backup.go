package collector

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Pre-compiled regex patterns for backup log parsing (optimization: compile once, not per scrape)
var (
	backupFinishedRe = regexp.MustCompile(`Finished Backup of VM (\d+)`)
	backupTimeRe     = regexp.MustCompile(`Backup finished at (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)
)

// batchJob represents a batch backup job that needs log parsing
type batchJob struct {
	UPID    string
	EndTime int64
}

// collectBackupMetricsWithGuests collects last backup timestamps for VMs and LXC containers
// OPTIMIZATION #2: Uses pre-fetched guest data from /cluster/resources to avoid duplicate API calls
// Also optimized with: parallel log fetches, early exit, dynamic log limits
func (c *ProxmoxCollector) collectBackupMetricsWithGuests(ch chan<- prometheus.Metric, nodes []string, guests map[string]GuestInfo) {
	// If no guests were passed (API failed), fall back to fetching ourselves
	if len(guests) == 0 {
		c.fetchGuestsFallback(nodes, guests)
	}

	// Now collect backup tasks and find latest successful backup per VMID
	backups := make(map[string]int64) // key: vmid, value: endtime timestamp
	var backupsMutex sync.Mutex

	// Count total guests for early exit optimization
	totalGuests := len(guests)

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			c.collectNodeBackups(nodeName, totalGuests, backups, &backupsMutex)
		}(node)
	}
	wg.Wait()

	// Emit metrics for each guest with a backup
	c.emitBackupMetrics(ch, backups, guests)
}

// fetchGuestsFallback fetches guest info when /cluster/resources failed
func (c *ProxmoxCollector) fetchGuestsFallback(nodes []string, guests map[string]GuestInfo) {
	var guestsMutex sync.Mutex
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			c.fetchNodeGuests(nodeName, guests, &guestsMutex)
		}(node)
	}
	wg.Wait()
}

// fetchNodeGuests fetches VMs and LXCs for a single node
func (c *ProxmoxCollector) fetchNodeGuests(nodeName string, guests map[string]GuestInfo, mu *sync.Mutex) {
	// Fetch VMs
	vmData, err := c.apiRequest(fmt.Sprintf("/nodes/%s/qemu", nodeName))
	if err == nil {
		var vmResult struct {
			Data []struct {
				VMID int64  `json:"vmid"`
				Name string `json:"name"`
			} `json:"data"`
		}
		if json.Unmarshal(vmData, &vmResult) == nil {
			mu.Lock()
			for _, vm := range vmResult.Data {
				vmid := strconv.FormatInt(vm.VMID, 10)
				guests[vmid] = GuestInfo{Node: nodeName, Name: vm.Name, Type: "qemu"}
			}
			mu.Unlock()
		}
	}

	// Fetch LXCs
	lxcData, err := c.apiRequest(fmt.Sprintf("/nodes/%s/lxc", nodeName))
	if err == nil {
		var lxcResult struct {
			Data []struct {
				VMID int64  `json:"vmid"`
				Name string `json:"name"`
			} `json:"data"`
		}
		if json.Unmarshal(lxcData, &lxcResult) == nil {
			mu.Lock()
			for _, lxc := range lxcResult.Data {
				vmid := strconv.FormatInt(lxc.VMID, 10)
				guests[vmid] = GuestInfo{Node: nodeName, Name: lxc.Name, Type: "lxc"}
			}
			mu.Unlock()
		}
	}
}

// collectNodeBackups collects backup info for a single node
func (c *ProxmoxCollector) collectNodeBackups(nodeName string, totalGuests int, backups map[string]int64, backupsMutex *sync.Mutex) {
	// Fetch vzdump tasks (limit 50 - recent backups are most relevant)
	tasksData, err := c.apiRequest(fmt.Sprintf("/nodes/%s/tasks?typefilter=vzdump&limit=50", nodeName))
	if err != nil {
		return
	}

	var tasksResult struct {
		Data []struct {
			ID        string `json:"id"`      // VMID as string (empty for batch jobs)
			UPID      string `json:"upid"`    // Unique task ID
			EndTime   int64  `json:"endtime"` // Unix timestamp
			Status    string `json:"status"`  // "OK" for successful
			StartTime int64  `json:"starttime"`
		} `json:"data"`
	}

	if err := json.Unmarshal(tasksData, &tasksResult); err != nil {
		return
	}

	var batchJobs []batchJob
	const maxBatchLogFetches = 5

	// First pass: collect single VM backups (fast) and identify batch jobs
	backupsMutex.Lock()
	for _, task := range tasksResult.Data {
		if task.Status != "OK" {
			continue
		}
		if task.ID != "" {
			// Single VM backup - use task data directly (fast path)
			if existing, ok := backups[task.ID]; !ok || task.EndTime > existing {
				backups[task.ID] = task.EndTime
			}
		} else if task.UPID != "" && len(batchJobs) < maxBatchLogFetches {
			batchJobs = append(batchJobs, batchJob{UPID: task.UPID, EndTime: task.EndTime})
		}
	}
	backupsMutex.Unlock()

	// Process batch jobs in parallel
	if len(batchJobs) > 0 {
		c.processBatchBackupJobs(nodeName, batchJobs, totalGuests, backups, backupsMutex)
	}
}

// processBatchBackupJobs processes batch backup jobs by parsing their logs
func (c *ProxmoxCollector) processBatchBackupJobs(nodeName string, batchJobs []batchJob, totalGuests int, backups map[string]int64, backupsMutex *sync.Mutex) {
	var batchWg sync.WaitGroup
	localBackups := make(map[string]int64)
	var localMutex sync.Mutex

	for _, job := range batchJobs {
		batchWg.Add(1)
		go func(upid string) {
			defer batchWg.Done()
			c.parseBackupLog(nodeName, upid, totalGuests, localBackups, &localMutex)
		}(job.UPID)
	}
	batchWg.Wait()

	// Merge local results into global backups map
	backupsMutex.Lock()
	for vmid, ts := range localBackups {
		if existing, ok := backups[vmid]; !ok || ts > existing {
			backups[vmid] = ts
		}
	}
	backupsMutex.Unlock()
}

// parseBackupLog parses a backup task log to extract VM backup timestamps
func (c *ProxmoxCollector) parseBackupLog(nodeName, upid string, totalGuests int, localBackups map[string]int64, localMutex *sync.Mutex) {
	// Fetch task log with high limit
	logData, err := c.apiRequest(fmt.Sprintf("/nodes/%s/tasks/%s/log?limit=1000000", nodeName, url.PathEscape(upid)))
	if err != nil {
		return
	}

	var logResult struct {
		Data []struct {
			N int    `json:"n"`
			T string `json:"t"`
		} `json:"data"`
	}

	if json.Unmarshal(logData, &logResult) != nil {
		return
	}

	// Parse log lines to find finished backups and their times
	var currentVMID string
	foundCount := 0
	for _, line := range logResult.Data {
		if match := backupFinishedRe.FindStringSubmatch(line.T); match != nil {
			currentVMID = match[1]
			continue
		}
		if currentVMID == "" || !strings.Contains(line.T, "Backup finished at") {
			continue
		}
		match := backupTimeRe.FindStringSubmatch(line.T)
		if match == nil {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", match[1])
		if err != nil {
			continue
		}
		timestamp := t.Unix()
		localMutex.Lock()
		if existing, ok := localBackups[currentVMID]; !ok || timestamp > existing {
			localBackups[currentVMID] = timestamp
			foundCount++
		}
		localMutex.Unlock()
		currentVMID = ""

		// Early exit if we found enough backups
		if foundCount >= totalGuests {
			break
		}
	}
}

// emitBackupMetrics emits Prometheus metrics for backup timestamps
func (c *ProxmoxCollector) emitBackupMetrics(ch chan<- prometheus.Metric, backups map[string]int64, guests map[string]GuestInfo) {
	for vmid, endtime := range backups {
		guest, ok := guests[vmid]
		if !ok {
			continue // Skip if we don't have guest info (maybe deleted)
		}
		labels := []string{guest.Node, vmid, guest.Name}
		if guest.Type == "qemu" {
			ch <- prometheus.MustNewConstMetric(c.vmLastBackup, prometheus.GaugeValue, float64(endtime), labels...)
		} else {
			ch <- prometheus.MustNewConstMetric(c.lxcLastBackup, prometheus.GaugeValue, float64(endtime), labels...)
		}
	}
}

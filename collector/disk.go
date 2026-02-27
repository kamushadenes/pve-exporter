package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	diskstatsPath = "/proc/diskstats"
)

// collectDiskMetrics collects disk SMART metrics via PVE API and local disk I/O
func (c *ProxmoxCollector) collectDiskMetrics(ch chan<- prometheus.Metric, nodes []string) {
	// Collect SMART metrics from PVE API for all nodes in parallel
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			c.collectNodeDiskSMART(ch, nodeName)
		}(node)
	}
	wg.Wait()

	// Collect local disk I/O metrics from /proc/diskstats (if available)
	hostname := getHostname()
	c.collectDiskIOMetrics(ch, hostname)
}

// collectDiskIOMetrics reads disk I/O stats from /proc/diskstats
func (c *ProxmoxCollector) collectDiskIOMetrics(ch chan<- prometheus.Metric, hostname string) {
	file, err := os.Open(diskstatsPath)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}

		device := fields[2]

		if isPartition(device) {
			continue
		}

		if strings.HasPrefix(device, "loop") || strings.HasPrefix(device, "ram") ||
			strings.HasPrefix(device, "zd") || strings.HasPrefix(device, "dm-") {
			continue
		}

		readsCompleted, _ := strconv.ParseFloat(fields[3], 64)
		sectorsRead, _ := strconv.ParseFloat(fields[5], 64)
		writesCompleted, _ := strconv.ParseFloat(fields[7], 64)
		sectorsWritten, _ := strconv.ParseFloat(fields[9], 64)
		ioTimeMs, _ := strconv.ParseFloat(fields[12], 64)

		readBytes := sectorsRead * 512
		writeBytes := sectorsWritten * 512
		ioTimeSeconds := ioTimeMs / 1000

		labels := []string{hostname, device}

		ch <- prometheus.MustNewConstMetric(c.diskReadBytes, prometheus.CounterValue, readBytes, labels...)
		ch <- prometheus.MustNewConstMetric(c.diskWriteBytes, prometheus.CounterValue, writeBytes, labels...)
		ch <- prometheus.MustNewConstMetric(c.diskReadsCompleted, prometheus.CounterValue, readsCompleted, labels...)
		ch <- prometheus.MustNewConstMetric(c.diskWritesCompleted, prometheus.CounterValue, writesCompleted, labels...)
		ch <- prometheus.MustNewConstMetric(c.diskIOTime, prometheus.CounterValue, ioTimeSeconds, labels...)
	}
}

// isPartition checks if a device name is a partition
func isPartition(device string) bool {
	if strings.Contains(device, "nvme") && strings.Contains(device, "p") {
		parts := strings.Split(device, "p")
		if len(parts) >= 2 {
			_, err := strconv.Atoi(parts[len(parts)-1])
			return err == nil
		}
	}

	if len(device) > 0 {
		lastChar := device[len(device)-1]
		if lastChar >= '0' && lastChar <= '9' {
			if !strings.HasPrefix(device, "nvme") {
				return true
			}
		}
	}

	return false
}

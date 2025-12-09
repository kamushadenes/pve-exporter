package collector

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Default path for SMART data JSON file
	smartDataPath = "/var/lib/pve-exporter/smart.json"
	// Maximum age of SMART data file before considering it stale (5 minutes)
	smartDataMaxAge = 5 * time.Minute
)

// SmartData represents the JSON structure from pve-smart-collector.sh
type SmartData struct {
	Hostname  string     `json:"hostname"`
	Timestamp int64      `json:"timestamp"`
	Disks     []DiskInfo `json:"disks"`
}

// DiskInfo represents a single disk's SMART data
type DiskInfo struct {
	Device                string  `json:"device"`
	Model                 string  `json:"model"`
	Serial                string  `json:"serial"`
	Type                  string  `json:"type"`
	Healthy               int     `json:"healthy"`
	Temperature           float64 `json:"temperature,omitempty"`
	PowerOnHours          float64 `json:"power_on_hours,omitempty"`
	DataWrittenBytes      float64 `json:"data_written_bytes,omitempty"`
	AvailableSparePercent float64 `json:"available_spare_percent,omitempty"`
	PercentageUsed        float64 `json:"percentage_used,omitempty"`
}

// collectDiskMetrics reads SMART data from JSON file and exports metrics
func (c *ProxmoxCollector) collectDiskMetrics(ch chan<- prometheus.Metric) {
	// Check if file exists
	info, err := os.Stat(smartDataPath)
	if os.IsNotExist(err) {
		// File doesn't exist - SMART collector not configured, skip silently
		return
	}
	if err != nil {
		log.Printf("Error checking SMART data file: %v", err)
		return
	}

	// Check if file is too old
	if time.Since(info.ModTime()) > smartDataMaxAge {
		log.Printf("SMART data file is stale (older than %v), skipping", smartDataMaxAge)
		return
	}

	// Read file
	data, err := os.ReadFile(smartDataPath)
	if err != nil {
		log.Printf("Error reading SMART data file: %v", err)
		return
	}

	// Parse JSON
	var smartData SmartData
	if err := json.Unmarshal(data, &smartData); err != nil {
		log.Printf("Error parsing SMART data JSON: %v", err)
		return
	}

	// Export metrics for each disk
	for _, disk := range smartData.Disks {
		labels := []string{smartData.Hostname, disk.Device, disk.Model, disk.Serial, disk.Type}

		// Health status (always present)
		ch <- prometheus.MustNewConstMetric(c.diskHealth, prometheus.GaugeValue, float64(disk.Healthy), labels...)

		// Temperature (if available)
		if disk.Temperature > 0 {
			ch <- prometheus.MustNewConstMetric(c.diskTemperature, prometheus.GaugeValue, disk.Temperature, labels...)
		}

		// Power on hours (if available)
		if disk.PowerOnHours > 0 {
			ch <- prometheus.MustNewConstMetric(c.diskPowerOnHours, prometheus.GaugeValue, disk.PowerOnHours, labels...)
		}

		// NVMe specific metrics
		if disk.DataWrittenBytes > 0 {
			ch <- prometheus.MustNewConstMetric(c.diskDataWritten, prometheus.CounterValue, disk.DataWrittenBytes, labels...)
		}

		if disk.AvailableSparePercent > 0 {
			ch <- prometheus.MustNewConstMetric(c.diskAvailableSpare, prometheus.GaugeValue, disk.AvailableSparePercent, labels...)
		}

		if disk.PercentageUsed >= 0 && disk.Type == "nvme" {
			ch <- prometheus.MustNewConstMetric(c.diskPercentageUsed, prometheus.GaugeValue, disk.PercentageUsed, labels...)
		}
	}
}

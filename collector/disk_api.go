package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	nvmeTemperatureRe    = regexp.MustCompile(`^Temperature:\s+(\d+)\s+Celsius`)
	nvmeAvailableSpareRe = regexp.MustCompile(`^Available Spare:\s+(\d+)%`)
	nvmePercentageUsedRe = regexp.MustCompile(`^Percentage Used:\s+(\d+)%`)
	nvmeDataWrittenRe    = regexp.MustCompile(`^Data Units Written:\s+([\d,]+)`)
	nvmePowerOnHoursRe   = regexp.MustCompile(`^Power On Hours:\s+([\d,]+)`)
)

// ataSmartAttr represents an ATA SMART attribute from PVE API
type ataSmartAttr struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Value      float64 `json:"value"`
	Worst      float64 `json:"worst"`
	Threshold  float64 `json:"threshold"`
	Raw        string  `json:"raw"`
	Normalized float64 `json:"normalized"`
}

// pveDisk represents a disk from the PVE API /nodes/{node}/disks/list
type pveDisk struct {
	DevPath string  `json:"devpath"`
	Serial  string  `json:"serial"`
	Model   string  `json:"model"`
	Type    string  `json:"type"`
	Health  string  `json:"health"`
	Wearout float64 `json:"wearout"`
	Size    float64 `json:"size"`
}

// pveSmartResponse represents the PVE API /nodes/{node}/disks/smart response
type pveSmartResponse struct {
	Data struct {
		Type       string         `json:"type"`
		Text       string         `json:"text"`
		Health     string         `json:"health"`
		Wearout    float64        `json:"wearout"`
		Attributes []ataSmartAttr `json:"attributes"`
	} `json:"data"`
}

// parseFirstNumber extracts the first number from a string like "31 (Min/Max 21/39)"
func parseFirstNumber(s string) float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return -1
	}
	return v
}

// collectNodeDiskSMART fetches disk list and SMART data for a single node via PVE API
func (c *ProxmoxCollector) collectNodeDiskSMART(ch chan<- prometheus.Metric, nodeName string) {
	path := fmt.Sprintf("/nodes/%s/disks/list", nodeName)
	data, err := c.apiRequest(path)
	if err != nil {
		log.Printf("Error fetching disk list for node %s: %v", nodeName, err)
		return
	}

	var result struct {
		Data []pveDisk `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling disk list for node %s: %v", nodeName, err)
		return
	}

	var wg sync.WaitGroup
	for _, disk := range result.Data {
		labels := []string{nodeName, disk.DevPath, disk.Model, disk.Serial, disk.Type}

		// Emit health from disk list
		healthVal := 0.0
		if disk.Health == "PASSED" {
			healthVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.diskHealth, prometheus.GaugeValue, healthVal, labels...)

		// Emit percentage used from wearout (wearout = remaining life %)
		if disk.Wearout >= 0 {
			ch <- prometheus.MustNewConstMetric(c.diskPercentageUsed, prometheus.GaugeValue, 100.0-disk.Wearout, labels...)
		}

		// Fetch detailed SMART data
		wg.Add(1)
		go func(d pveDisk) {
			defer wg.Done()
			c.collectDiskSmartDetail(ch, nodeName, d)
		}(disk)
	}
	wg.Wait()
}

// collectDiskSmartDetail fetches SMART details for a single disk via PVE API
func (c *ProxmoxCollector) collectDiskSmartDetail(ch chan<- prometheus.Metric, nodeName string, disk pveDisk) {
	path := fmt.Sprintf("/nodes/%s/disks/smart?disk=%s", nodeName, disk.DevPath)
	data, err := c.apiRequest(path)
	if err != nil {
		return
	}

	var resp pveSmartResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	labels := []string{nodeName, disk.DevPath, disk.Model, disk.Serial, disk.Type}

	switch resp.Data.Type {
	case "text":
		c.parseNVMeSmartText(ch, resp.Data.Text, labels)
	case "ata":
		c.parseATASmartAttrs(ch, resp.Data.Attributes, labels)
	}
}

// parseATASmartAttrs parses ATA SMART structured attributes into Prometheus metrics
func (c *ProxmoxCollector) parseATASmartAttrs(ch chan<- prometheus.Metric, attrs []ataSmartAttr, labels []string) {
	for _, attr := range attrs {
		raw := strings.TrimSpace(attr.Raw)

		switch attr.Name {
		case "Temperature_Celsius", "Drive_Temperature":
			if v := parseFirstNumber(raw); v >= 0 {
				ch <- prometheus.MustNewConstMetric(c.diskTemperature, prometheus.GaugeValue, v, labels...)
			}
		case "Power_On_Hours":
			if v := parseFirstNumber(raw); v >= 0 {
				ch <- prometheus.MustNewConstMetric(c.diskPowerOnHours, prometheus.GaugeValue, v, labels...)
			}
		case "Host_Writes_32MiB":
			if strings.TrimSpace(attr.ID) == "241" {
				if v := parseFirstNumber(raw); v >= 0 {
					ch <- prometheus.MustNewConstMetric(c.diskDataWritten, prometheus.GaugeValue, v*32*1024*1024, labels...)
				}
			}
		case "Available_Reservd_Space":
			ch <- prometheus.MustNewConstMetric(c.diskAvailableSpare, prometheus.GaugeValue, attr.Normalized, labels...)
		case "Media_Wearout_Indicator":
			ch <- prometheus.MustNewConstMetric(c.diskPercentageUsed, prometheus.GaugeValue, 100-attr.Normalized, labels...)
		}
	}
}

// parseNVMeSmartText parses NVMe SMART text output into Prometheus metrics
func (c *ProxmoxCollector) parseNVMeSmartText(ch chan<- prometheus.Metric, text string, labels []string) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if m := nvmeTemperatureRe.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				ch <- prometheus.MustNewConstMetric(c.diskTemperature, prometheus.GaugeValue, v, labels...)
			}
		}
		if m := nvmeAvailableSpareRe.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				ch <- prometheus.MustNewConstMetric(c.diskAvailableSpare, prometheus.GaugeValue, v, labels...)
			}
		}
		if m := nvmePercentageUsedRe.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				ch <- prometheus.MustNewConstMetric(c.diskPercentageUsed, prometheus.GaugeValue, v, labels...)
			}
		}
		if m := nvmeDataWrittenRe.FindStringSubmatch(line); m != nil {
			raw := strings.ReplaceAll(m[1], ",", "")
			if units, err := strconv.ParseFloat(raw, 64); err == nil {
				ch <- prometheus.MustNewConstMetric(c.diskDataWritten, prometheus.GaugeValue, units*524288, labels...)
			}
		}
		if m := nvmePowerOnHoursRe.FindStringSubmatch(line); m != nil {
			raw := strings.ReplaceAll(m[1], ",", "")
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				ch <- prometheus.MustNewConstMetric(c.diskPowerOnHours, prometheus.GaugeValue, v, labels...)
			}
		}
	}
}

package collector

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// cachedHostname stores hostname to avoid repeated os.Hostname() calls
var (
	cachedHostname     string
	cachedHostnameOnce sync.Once
)

func getHostname() string {
	cachedHostnameOnce.Do(func() {
		cachedHostname, _ = os.Hostname()
		if cachedHostname == "" {
			cachedHostname = "localhost"
		}
	})
	return cachedHostname
}

// sensorReading holds a sensor reading with its type
type sensorReading struct {
	value      float64
	metricType string // "temp", "fan", "in", "power"
}

// collectSensorsMetrics collects hardware sensor metrics using lm-sensors
func (c *ProxmoxCollector) collectSensorsMetrics(ch chan<- prometheus.Metric) {
	// Execute sensors -j for JSON output
	cmd := exec.Command("sensors", "-j")
	output, err := cmd.Output()
	if err != nil {
		// sensors might not be installed or no sensors detected
		return
	}

	hostname := getHostname()

	// Parse JSON output
	var sensorsData map[string]interface{}
	if err := json.Unmarshal(output, &sensorsData); err != nil {
		log.Printf("Error parsing sensors JSON: %v", err)
		return
	}

	// Iterate over each chip
	for chipName, chipData := range sensorsData {
		chipMap, ok := chipData.(map[string]interface{})
		if !ok {
			continue
		}

		// Get adapter name
		adapter := ""
		if adapterVal, ok := chipMap["Adapter"].(string); ok {
			adapter = adapterVal
		}

		// Iterate over sensors in this chip
		for sensorName, sensorData := range chipMap {
			if sensorName == "Adapter" {
				continue
			}

			sensorMap, ok := sensorData.(map[string]interface{})
			if !ok {
				continue
			}

			// Collect readings for this sensor, preferring _input over _average
			readings := make(map[string]*sensorReading)

			for key, value := range sensorMap {
				val, ok := value.(float64)
				if !ok {
					continue
				}

				// Determine metric type from key prefix/suffix
				switch {
				case strings.HasPrefix(key, "temp") && strings.HasSuffix(key, "_input"):
					readings["temp"] = &sensorReading{value: val, metricType: "temp"}
				case strings.HasPrefix(key, "fan") && strings.HasSuffix(key, "_input"):
					readings["fan"] = &sensorReading{value: val, metricType: "fan"}
				case strings.HasPrefix(key, "in") && strings.HasSuffix(key, "_input"):
					readings["in"] = &sensorReading{value: val, metricType: "in"}
				case strings.HasPrefix(key, "power") && strings.HasSuffix(key, "_input"):
					// power_input has priority over power_average
					readings["power"] = &sensorReading{value: val, metricType: "power"}
				case strings.HasPrefix(key, "power") && strings.HasSuffix(key, "_average"):
					// Only use power_average if power_input not present
					if _, exists := readings["power"]; !exists {
						readings["power"] = &sensorReading{value: val, metricType: "power"}
					}
				}
			}

			// Emit metrics
			for _, reading := range readings {
				switch reading.metricType {
				case "temp":
					ch <- prometheus.MustNewConstMetric(
						c.sensorTemperature,
						prometheus.GaugeValue,
						reading.value,
						hostname, chipName, adapter, sensorName,
					)
				case "fan":
					ch <- prometheus.MustNewConstMetric(
						c.sensorFanRPM,
						prometheus.GaugeValue,
						reading.value,
						hostname, chipName, adapter, sensorName,
					)
				case "in":
					ch <- prometheus.MustNewConstMetric(
						c.sensorVoltage,
						prometheus.GaugeValue,
						reading.value,
						hostname, chipName, adapter, sensorName,
					)
				case "power":
					ch <- prometheus.MustNewConstMetric(
						c.sensorPower,
						prometheus.GaugeValue,
						reading.value,
						hostname, chipName, adapter, sensorName,
					)
				}
			}
		}
	}
}

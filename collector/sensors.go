package collector

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

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

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

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

				var metricType string
				var baseKey string

				if strings.HasPrefix(key, "temp") && strings.HasSuffix(key, "_input") {
					metricType = "temp"
					baseKey = "temp"
				} else if strings.HasPrefix(key, "fan") && strings.HasSuffix(key, "_input") {
					metricType = "fan"
					baseKey = "fan"
				} else if strings.HasPrefix(key, "in") && strings.HasSuffix(key, "_input") {
					metricType = "in"
					baseKey = "in"
				} else if strings.HasPrefix(key, "power") {
					if strings.HasSuffix(key, "_input") {
						metricType = "power"
						baseKey = "power_input"
					} else if strings.HasSuffix(key, "_average") {
						metricType = "power"
						baseKey = "power_average"
					}
				} else {
					continue
				}

				if metricType == "" {
					continue
				}

				// For power, prefer _input over _average
				if metricType == "power" {
					if existing, exists := readings["power"]; exists {
						// Only overwrite if this is _input (higher priority)
						if baseKey == "power_input" {
							existing.value = val
						}
					} else {
						readings["power"] = &sensorReading{value: val, metricType: metricType}
					}
				} else {
					readings[metricType] = &sensorReading{value: val, metricType: metricType}
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

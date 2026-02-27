package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bigtcze/pve-exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// collectMetrics is a test helper that drains a metrics channel into a slice
func collectMetrics(ch chan prometheus.Metric) []prometheus.Metric {
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	return metrics
}

// getMetricValue extracts the float value from a prometheus.Metric
func getMetricValue(m prometheus.Metric) float64 {
	pb := &dto.Metric{}
	_ = m.Write(pb)
	if pb.Gauge != nil {
		return pb.GetGauge().GetValue()
	}
	if pb.Counter != nil {
		return pb.GetCounter().GetValue()
	}
	return 0
}

func TestParseNVMeSmartText(t *testing.T) {
	cfg := &config.ProxmoxConfig{Host: "localhost", User: "root@pam"}
	c := NewProxmoxCollector(cfg)

	nvmeText := `
SMART/Health Information (NVMe Log 0x02)
Critical Warning:                   0x00
Temperature:                        71 Celsius
Available Spare:                    100%
Available Spare Threshold:          10%
Percentage Used:                    37%
Data Units Read:                    15,079,412 [7.72 TB]
Data Units Written:                 124,766,241 [63.8 TB]
Host Read Commands:                 308,099,076
Host Write Commands:                3,879,578,704
Controller Busy Time:               9,122
Power Cycles:                       166
Power On Hours:                     9,672
Unsafe Shutdowns:                   136
Media and Data Integrity Errors:    0
Error Information Log Entries:      443
Warning  Comp. Temperature Time:    0
Critical Comp. Temperature Time:    441
Temperature Sensor 1:               71 Celsius
Temperature Sensor 2:               76 Celsius
`

	labels := []string{"pve1", "/dev/nvme0n1", "Samsung SSD 970 EVO Plus 250GB", "S59BNM0RB04658M", "nvme"}
	ch := make(chan prometheus.Metric, 100)

	c.parseNVMeSmartText(ch, nvmeText, labels)
	close(ch)

	metrics := collectMetrics(ch)

	// We expect: temperature, available spare, percentage used, data written, power on hours
	if len(metrics) != 5 {
		t.Fatalf("expected 5 metrics, got %d", len(metrics))
	}

	// Check temperature = 71
	if v := getMetricValue(metrics[0]); v != 71 {
		t.Errorf("expected temperature 71, got %f", v)
	}

	// Check available spare = 100
	if v := getMetricValue(metrics[1]); v != 100 {
		t.Errorf("expected available spare 100, got %f", v)
	}

	// Check percentage used = 37
	if v := getMetricValue(metrics[2]); v != 37 {
		t.Errorf("expected percentage used 37, got %f", v)
	}

	// Check data written = 124,766,241 * 524288 bytes
	expectedDataWritten := 124766241.0 * 524288
	if v := getMetricValue(metrics[3]); v != expectedDataWritten {
		t.Errorf("expected data written %f, got %f", expectedDataWritten, v)
	}

	// Check power on hours = 9672
	if v := getMetricValue(metrics[4]); v != 9672 {
		t.Errorf("expected power on hours 9672, got %f", v)
	}
}

func TestParseATASmartAttrs(t *testing.T) {
	cfg := &config.ProxmoxConfig{Host: "localhost", User: "root@pam"}
	c := NewProxmoxCollector(cfg)

	attrs := []ataSmartAttr{
		{ID: "194", Name: "Temperature_Celsius", Raw: "31", Normalized: 100},
		{ID: "  9", Name: "Power_On_Hours", Raw: "58046", Normalized: 100},
		{ID: "241", Name: "Host_Writes_32MiB", Raw: "43161060", Normalized: 100},
		{ID: "170", Name: "Available_Reservd_Space", Raw: "1", Normalized: 99},
		{ID: "233", Name: "Media_Wearout_Indicator", Raw: "0", Normalized: 93},
	}

	labels := []string{"pve1", "/dev/sda", "INTEL_SSDSC2KG019T8", "PHYG830600161P9DGN", "ssd"}
	ch := make(chan prometheus.Metric, 100)

	c.parseATASmartAttrs(ch, attrs, labels)
	close(ch)

	metrics := collectMetrics(ch)

	// Expect: temperature, power on hours, data written, available spare, percentage used
	if len(metrics) != 5 {
		t.Fatalf("expected 5 metrics, got %d", len(metrics))
	}

	// Temperature = 31
	if v := getMetricValue(metrics[0]); v != 31 {
		t.Errorf("expected temperature 31, got %f", v)
	}

	// Power on hours = 58046
	if v := getMetricValue(metrics[1]); v != 58046 {
		t.Errorf("expected power on hours 58046, got %f", v)
	}

	// Data written = 43161060 * 32 * 1024 * 1024 bytes
	expectedWritten := 43161060.0 * 32 * 1024 * 1024
	if v := getMetricValue(metrics[2]); v != expectedWritten {
		t.Errorf("expected data written %f, got %f", expectedWritten, v)
	}

	// Available spare = 99 (normalized)
	if v := getMetricValue(metrics[3]); v != 99 {
		t.Errorf("expected available spare 99, got %f", v)
	}

	// Percentage used = 100 - 93 = 7
	if v := getMetricValue(metrics[4]); v != 7 {
		t.Errorf("expected percentage used 7, got %f", v)
	}
}

func TestCollectNodeDiskSMART(t *testing.T) {
	// Mock PVE API server
	mux := http.NewServeMux()

	// Disk list endpoint
	diskList := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"devpath": "/dev/nvme0n1",
				"serial":  "S59BNM0RB04658M",
				"model":   "Samsung SSD 970 EVO Plus 250GB",
				"type":    "nvme",
				"health":  "PASSED",
				"wearout": 63,
				"size":    250059350016,
			},
		},
	}
	mux.HandleFunc("/api2/json/nodes/pve1/disks/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(diskList)
	})

	// SMART endpoint for the NVMe disk
	smartResp := map[string]interface{}{
		"data": map[string]interface{}{
			"type":    "text",
			"health":  "PASSED",
			"wearout": 63,
			"text":    "\nTemperature:                        71 Celsius\nAvailable Spare:                    100%\nPercentage Used:                    37%\nData Units Written:                 124,766,241 [63.8 TB]\nPower On Hours:                     9,672\n",
		},
	}
	mux.HandleFunc("/api2/json/nodes/pve1/disks/smart", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(smartResp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	// Parse host:port from server URL
	hostPort := strings.TrimPrefix(server.URL, "https://")
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	port := 443
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	cfg := &config.ProxmoxConfig{
		Host:               host,
		Port:               port,
		TokenID:            "test@pve!test",
		TokenSecret:        "test-secret",
		InsecureSkipVerify: true,
	}
	c := NewProxmoxCollector(cfg)
	c.client = server.Client()

	ch := make(chan prometheus.Metric, 100)
	c.collectNodeDiskSMART(ch, "pve1")
	close(ch)

	metrics := collectMetrics(ch)

	// Expect: health (from disk list) + percentage used (from wearout) + 5 SMART metrics
	// = 7 total
	if len(metrics) < 7 {
		t.Fatalf("expected at least 7 metrics, got %d", len(metrics))
	}

	// First metric should be disk health = 1 (PASSED)
	if v := getMetricValue(metrics[0]); v != 1 {
		t.Errorf("expected health 1, got %f", v)
	}

	// Second metric should be percentage used from wearout = 100 - 63 = 37
	if v := getMetricValue(metrics[1]); v != 37 {
		t.Errorf("expected percentage used from wearout 37, got %f", v)
	}
}

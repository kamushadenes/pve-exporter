package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// collectVMMetricsWithNodes collects VM and container metrics using pre-fetched nodes list
func (c *ProxmoxCollector) collectVMMetricsWithNodes(ch chan<- prometheus.Metric, nodes []string) {
	// Process all nodes in parallel for better performance
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			// QEMU VMs
			vmCount := c.collectResourceMetrics(ch, nodeName, "qemu")
			ch <- prometheus.MustNewConstMetric(c.nodeVMCount, prometheus.GaugeValue, float64(vmCount), nodeName)

			// LXC containers
			lxcCount := c.collectResourceMetrics(ch, nodeName, "lxc")
			ch <- prometheus.MustNewConstMetric(c.nodeLXCCount, prometheus.GaugeValue, float64(lxcCount), nodeName)
		}(node)
	}
	wg.Wait()
}

// collectResourceMetrics collects metrics for VMs or containers and returns the count
func (c *ProxmoxCollector) collectResourceMetrics(ch chan<- prometheus.Metric, node, resType string) int {
	path := fmt.Sprintf("/nodes/%s/%s", node, resType)
	data, err := c.apiRequest(path)
	if err != nil {
		log.Printf("Error fetching %s for node %s: %v", resType, node, err)
		return 0
	}

	var result struct {
		Data []struct {
			VMID      int64   `json:"vmid"`
			Name      string  `json:"name"`
			Status    string  `json:"status"`
			Uptime    float64 `json:"uptime"`
			CPU       float64 `json:"cpu"`
			CPUs      float64 `json:"cpus"`
			Mem       float64 `json:"mem"`
			MaxMem    float64 `json:"maxmem"`
			Disk      float64 `json:"disk"`
			MaxDisk   float64 `json:"maxdisk"`
			NetIn     float64 `json:"netin"`
			NetOut    float64 `json:"netout"`
			DiskRead  float64 `json:"diskread"`
			DiskWrite float64 `json:"diskwrite"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Error unmarshaling %s for node %s: %v", resType, node, err)
		return 0
	}

	// Process VMs in parallel for better performance
	var wg sync.WaitGroup

	for _, vm := range result.Data {
		wg.Add(1)
		go func(vm struct {
			VMID      int64   `json:"vmid"`
			Name      string  `json:"name"`
			Status    string  `json:"status"`
			Uptime    float64 `json:"uptime"`
			CPU       float64 `json:"cpu"`
			CPUs      float64 `json:"cpus"`
			Mem       float64 `json:"mem"`
			MaxMem    float64 `json:"maxmem"`
			Disk      float64 `json:"disk"`
			MaxDisk   float64 `json:"maxdisk"`
			NetIn     float64 `json:"netin"`
			NetOut    float64 `json:"netout"`
			DiskRead  float64 `json:"diskread"`
			DiskWrite float64 `json:"diskwrite"`
		}) {
			defer wg.Done()

			status := 0.0
			if vm.Status == "running" {
				status = 1.0
			}

			labels := []string{node, fmt.Sprintf("%d", vm.VMID), vm.Name}

			// Get detailed status ONCE for all metrics (disk I/O, balloon, pressure, etc.)
			diskRead := vm.DiskRead
			diskWrite := vm.DiskWrite
			var detailData []byte
			if vm.Status == "running" {
				detailPath := fmt.Sprintf("/nodes/%s/%s/%d/status/current", node, resType, vm.VMID)
				var err error
				detailData, err = c.apiRequest(detailPath)
				if err == nil {
					var detailResult struct {
						Data struct {
							DiskRead  float64 `json:"diskread"`
							DiskWrite float64 `json:"diskwrite"`
						} `json:"data"`
					}
					if err := json.Unmarshal(detailData, &detailResult); err == nil {
						diskRead = detailResult.Data.DiskRead
						diskWrite = detailResult.Data.DiskWrite
					}
				}
			}

			if resType == "lxc" {
				ch <- prometheus.MustNewConstMetric(c.lxcStatus, prometheus.GaugeValue, status, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcUptime, prometheus.GaugeValue, vm.Uptime, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcCPU, prometheus.GaugeValue, vm.CPU, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcCPUs, prometheus.GaugeValue, vm.CPUs, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcMemory, prometheus.GaugeValue, vm.Mem, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcMaxMemory, prometheus.GaugeValue, vm.MaxMem, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcDisk, prometheus.GaugeValue, vm.Disk, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcMaxDisk, prometheus.GaugeValue, vm.MaxDisk, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcNetIn, prometheus.CounterValue, vm.NetIn, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcNetOut, prometheus.CounterValue, vm.NetOut, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcDiskRead, prometheus.CounterValue, diskRead, labels...)
				ch <- prometheus.MustNewConstMetric(c.lxcDiskWrite, prometheus.CounterValue, diskWrite, labels...)
				// Get LXC swap - reuse detailData if available
				c.collectLXCSwapMetricsFromData(ch, detailData, labels)
			} else {
				ch <- prometheus.MustNewConstMetric(c.vmStatus, prometheus.GaugeValue, status, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmUptime, prometheus.GaugeValue, vm.Uptime, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmCPU, prometheus.GaugeValue, vm.CPU, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmCPUs, prometheus.GaugeValue, vm.CPUs, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmMemory, prometheus.GaugeValue, vm.Mem, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmMaxMemory, prometheus.GaugeValue, vm.MaxMem, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmMaxDisk, prometheus.GaugeValue, vm.MaxDisk, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmNetIn, prometheus.CounterValue, vm.NetIn, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmNetOut, prometheus.CounterValue, vm.NetOut, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmDiskRead, prometheus.CounterValue, diskRead, labels...)
				ch <- prometheus.MustNewConstMetric(c.vmDiskWrite, prometheus.CounterValue, diskWrite, labels...)
				// Get VM detailed metrics - reuse detailData instead of making another API call
				c.collectVMDetailedMetricsFromData(ch, detailData, labels)
			}
		}(vm)
	}

	wg.Wait()
	return len(result.Data)
}

// collectLXCSwapMetricsFromData parses LXC swap metrics from already fetched data
func (c *ProxmoxCollector) collectLXCSwapMetricsFromData(ch chan<- prometheus.Metric, data []byte, labels []string) {
	if data == nil {
		return
	}

	var result struct {
		Data struct {
			Swap    float64 `json:"swap"`
			MaxSwap float64 `json:"maxswap"`
			PID     float64 `json:"pid"`
			HA      struct {
				Managed int `json:"managed"`
			} `json:"ha"`
			PressureCPUFull    string `json:"pressurecpufull"`
			PressureCPUSome    string `json:"pressurecpusome"`
			PressureIOFull     string `json:"pressureiofull"`
			PressureIOSome     string `json:"pressureiosome"`
			PressureMemoryFull string `json:"pressurememoryfull"`
			PressureMemorySome string `json:"pressurememorysome"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(c.lxcSwap, prometheus.GaugeValue, result.Data.Swap, labels...)
	ch <- prometheus.MustNewConstMetric(c.lxcMaxSwap, prometheus.GaugeValue, result.Data.MaxSwap, labels...)
	ch <- prometheus.MustNewConstMetric(c.lxcHAManaged, prometheus.GaugeValue, float64(result.Data.HA.Managed), labels...)
	ch <- prometheus.MustNewConstMetric(c.lxcPID, prometheus.GaugeValue, result.Data.PID, labels...)
	// Pressure metrics (LXC returns strings)
	if cpuFull, err := strconv.ParseFloat(result.Data.PressureCPUFull, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureCPUFull, prometheus.GaugeValue, cpuFull, labels...)
	}
	if cpuSome, err := strconv.ParseFloat(result.Data.PressureCPUSome, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureCPUSome, prometheus.GaugeValue, cpuSome, labels...)
	}
	if ioFull, err := strconv.ParseFloat(result.Data.PressureIOFull, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureIOFull, prometheus.GaugeValue, ioFull, labels...)
	}
	if ioSome, err := strconv.ParseFloat(result.Data.PressureIOSome, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureIOSome, prometheus.GaugeValue, ioSome, labels...)
	}
	if memFull, err := strconv.ParseFloat(result.Data.PressureMemoryFull, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureMemoryFull, prometheus.GaugeValue, memFull, labels...)
	}
	if memSome, err := strconv.ParseFloat(result.Data.PressureMemorySome, 64); err == nil {
		ch <- prometheus.MustNewConstMetric(c.lxcPressureMemorySome, prometheus.GaugeValue, memSome, labels...)
	}
}

// collectVMDetailedMetricsFromData parses VM detailed metrics from already fetched data
func (c *ProxmoxCollector) collectVMDetailedMetricsFromData(ch chan<- prometheus.Metric, data []byte, labels []string) {
	if data == nil {
		return
	}

	var result struct {
		Data struct {
			Balloon float64 `json:"balloon"`
			FreeMem float64 `json:"freemem"`
			PID     float64 `json:"pid"`
			MemHost float64 `json:"memhost"`
			HA      struct {
				Managed int `json:"managed"`
			} `json:"ha"`
			BalloonInfo struct {
				Actual          float64 `json:"actual"`
				MaxMem          float64 `json:"max_mem"`
				TotalMem        float64 `json:"total_mem"`
				MajorPageFaults float64 `json:"major_page_faults"`
				MinorPageFaults float64 `json:"minor_page_faults"`
				MemSwappedIn    float64 `json:"mem_swapped_in"`
				MemSwappedOut   float64 `json:"mem_swapped_out"`
			} `json:"ballooninfo"`
			PressureCPUFull    float64 `json:"pressurecpufull"`
			PressureCPUSome    float64 `json:"pressurecpusome"`
			PressureIOFull     float64 `json:"pressureiofull"`
			PressureIOSome     float64 `json:"pressureiosome"`
			PressureMemoryFull float64 `json:"pressurememoryfull"`
			PressureMemorySome float64 `json:"pressurememorysome"`
			BlockStat          map[string]struct {
				RdBytes     float64 `json:"rd_bytes"`
				WrBytes     float64 `json:"wr_bytes"`
				RdOps       float64 `json:"rd_operations"`
				WrOps       float64 `json:"wr_operations"`
				FailedRdOps float64 `json:"failed_rd_operations"`
				FailedWrOps float64 `json:"failed_wr_operations"`
				FlushOps    float64 `json:"flush_operations"`
			} `json:"blockstat"`
			NICS map[string]struct {
				NetIn  float64 `json:"netin"`
				NetOut float64 `json:"netout"`
			} `json:"nics"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(c.vmBalloon, prometheus.GaugeValue, result.Data.Balloon, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmFreeMem, prometheus.GaugeValue, result.Data.FreeMem, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmHAManaged, prometheus.GaugeValue, float64(result.Data.HA.Managed), labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPID, prometheus.GaugeValue, result.Data.PID, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmMemHost, prometheus.GaugeValue, result.Data.MemHost, labels...)
	// Pressure metrics
	ch <- prometheus.MustNewConstMetric(c.vmPressureCPUFull, prometheus.GaugeValue, result.Data.PressureCPUFull, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPressureCPUSome, prometheus.GaugeValue, result.Data.PressureCPUSome, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPressureIOFull, prometheus.GaugeValue, result.Data.PressureIOFull, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPressureIOSome, prometheus.GaugeValue, result.Data.PressureIOSome, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPressureMemoryFull, prometheus.GaugeValue, result.Data.PressureMemoryFull, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmPressureMemorySome, prometheus.GaugeValue, result.Data.PressureMemorySome, labels...)
	// Balloon info
	ch <- prometheus.MustNewConstMetric(c.vmBalloonActual, prometheus.GaugeValue, result.Data.BalloonInfo.Actual, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonMaxMem, prometheus.GaugeValue, result.Data.BalloonInfo.MaxMem, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonTotalMem, prometheus.GaugeValue, result.Data.BalloonInfo.TotalMem, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonMajorFaults, prometheus.CounterValue, result.Data.BalloonInfo.MajorPageFaults, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonMinorFaults, prometheus.CounterValue, result.Data.BalloonInfo.MinorPageFaults, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonMemSwappedIn, prometheus.GaugeValue, result.Data.BalloonInfo.MemSwappedIn, labels...)
	ch <- prometheus.MustNewConstMetric(c.vmBalloonMemSwappedOut, prometheus.GaugeValue, result.Data.BalloonInfo.MemSwappedOut, labels...)

	// Block device metrics
	for device, stats := range result.Data.BlockStat {
		deviceLabels := append(labels, device)
		ch <- prometheus.MustNewConstMetric(c.vmBlockReadBytes, prometheus.CounterValue, stats.RdBytes, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockWriteBytes, prometheus.CounterValue, stats.WrBytes, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockReadOps, prometheus.CounterValue, stats.RdOps, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockWriteOps, prometheus.CounterValue, stats.WrOps, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockFailedRead, prometheus.CounterValue, stats.FailedRdOps, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockFailedWrite, prometheus.CounterValue, stats.FailedWrOps, deviceLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmBlockFlushOps, prometheus.CounterValue, stats.FlushOps, deviceLabels...)
	}

	// NIC metrics
	for iface, stats := range result.Data.NICS {
		nicLabels := append(labels, iface)
		ch <- prometheus.MustNewConstMetric(c.vmNICNetIn, prometheus.CounterValue, stats.NetIn, nicLabels...)
		ch <- prometheus.MustNewConstMetric(c.vmNICNetOut, prometheus.CounterValue, stats.NetOut, nicLabels...)
	}
}

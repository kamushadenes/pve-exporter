package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// ZFSCollector collects ZFS metrics from kstat
type ZFSCollector struct {
	kstatPath string

	// ARC metrics
	arcSize           *prometheus.Desc
	arcCMax           *prometheus.Desc
	arcCMin           *prometheus.Desc
	arcC              *prometheus.Desc
	arcP              *prometheus.Desc
	arcHits           *prometheus.Desc
	arcMisses         *prometheus.Desc
	arcTargetSize     *prometheus.Desc
	arcMRUSize        *prometheus.Desc
	arcMFUSize        *prometheus.Desc
	arcMRUGhostSize   *prometheus.Desc
	arcMFUGhostSize   *prometheus.Desc
	arcDataSize       *prometheus.Desc
	arcMetadataSize   *prometheus.Desc
	arcEvictSkip      *prometheus.Desc
	arcMemoryThrottle *prometheus.Desc

	// Pool metrics
	poolHealth        *prometheus.Desc
	poolCapacity      *prometheus.Desc
	poolAllocated     *prometheus.Desc
	poolFree          *prometheus.Desc
	poolFragmentation *prometheus.Desc
}

// NewZFSCollector creates a new ZFS collector
func NewZFSCollector(kstatPath string) *ZFSCollector {
	return &ZFSCollector{
		kstatPath: kstatPath,

		// ARC metrics
		arcSize: prometheus.NewDesc(
			"zfs_arc_size_bytes",
			"Current size of ARC in bytes",
			nil, nil,
		),
		arcCMax: prometheus.NewDesc(
			"zfs_arc_c_max_bytes",
			"Maximum size of ARC in bytes",
			nil, nil,
		),
		arcCMin: prometheus.NewDesc(
			"zfs_arc_c_min_bytes",
			"Minimum size of ARC in bytes",
			nil, nil,
		),
		arcC: prometheus.NewDesc(
			"zfs_arc_c_bytes",
			"Target size of ARC in bytes",
			nil, nil,
		),
		arcP: prometheus.NewDesc(
			"zfs_arc_p_bytes",
			"Target size of MRU in bytes",
			nil, nil,
		),
		arcHits: prometheus.NewDesc(
			"zfs_arc_hits_total",
			"Total number of ARC hits",
			nil, nil,
		),
		arcMisses: prometheus.NewDesc(
			"zfs_arc_misses_total",
			"Total number of ARC misses",
			nil, nil,
		),
		arcTargetSize: prometheus.NewDesc(
			"zfs_arc_target_size_bytes",
			"Target size of ARC",
			nil, nil,
		),
		arcMRUSize: prometheus.NewDesc(
			"zfs_arc_mru_size_bytes",
			"Size of MRU list in bytes",
			nil, nil,
		),
		arcMFUSize: prometheus.NewDesc(
			"zfs_arc_mfu_size_bytes",
			"Size of MFU list in bytes",
			nil, nil,
		),
		arcMRUGhostSize: prometheus.NewDesc(
			"zfs_arc_mru_ghost_size_bytes",
			"Size of MRU ghost list in bytes",
			nil, nil,
		),
		arcMFUGhostSize: prometheus.NewDesc(
			"zfs_arc_mfu_ghost_size_bytes",
			"Size of MFU ghost list in bytes",
			nil, nil,
		),
		arcDataSize: prometheus.NewDesc(
			"zfs_arc_data_size_bytes",
			"Size of data in ARC in bytes",
			nil, nil,
		),
		arcMetadataSize: prometheus.NewDesc(
			"zfs_arc_metadata_size_bytes",
			"Size of metadata in ARC in bytes",
			nil, nil,
		),
		arcEvictSkip: prometheus.NewDesc(
			"zfs_arc_evict_skip_total",
			"Total number of evictions skipped",
			nil, nil,
		),
		arcMemoryThrottle: prometheus.NewDesc(
			"zfs_arc_memory_throttle_count_total",
			"Total number of memory throttle events",
			nil, nil,
		),

		// Pool metrics (will be labeled by pool name)
		poolHealth: prometheus.NewDesc(
			"zfs_pool_health",
			"Health status of ZFS pool (0=degraded, 1=online)",
			[]string{"pool"}, nil,
		),
		poolCapacity: prometheus.NewDesc(
			"zfs_pool_capacity_bytes",
			"Total capacity of ZFS pool in bytes",
			[]string{"pool"}, nil,
		),
		poolAllocated: prometheus.NewDesc(
			"zfs_pool_allocated_bytes",
			"Allocated space in ZFS pool in bytes",
			[]string{"pool"}, nil,
		),
		poolFree: prometheus.NewDesc(
			"zfs_pool_free_bytes",
			"Free space in ZFS pool in bytes",
			[]string{"pool"}, nil,
		),
		poolFragmentation: prometheus.NewDesc(
			"zfs_pool_fragmentation_percent",
			"Fragmentation percentage of ZFS pool",
			[]string{"pool"}, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *ZFSCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.arcSize
	ch <- c.arcCMax
	ch <- c.arcCMin
	ch <- c.arcC
	ch <- c.arcP
	ch <- c.arcHits
	ch <- c.arcMisses
	ch <- c.arcTargetSize
	ch <- c.arcMRUSize
	ch <- c.arcMFUSize
	ch <- c.arcMRUGhostSize
	ch <- c.arcMFUGhostSize
	ch <- c.arcDataSize
	ch <- c.arcMetadataSize
	ch <- c.arcEvictSkip
	ch <- c.arcMemoryThrottle
	ch <- c.poolHealth
	ch <- c.poolCapacity
	ch <- c.poolAllocated
	ch <- c.poolFree
	ch <- c.poolFragmentation
}

// Collect implements prometheus.Collector
func (c *ZFSCollector) Collect(ch chan<- prometheus.Metric) {
	// Collect ARC stats
	c.collectARCStats(ch)

	// Collect pool stats
	c.collectPoolStats(ch)
}

// collectARCStats reads and exports ARC statistics
func (c *ZFSCollector) collectARCStats(ch chan<- prometheus.Metric) {
	arcstatsPath := filepath.Join(c.kstatPath, "arcstats")
	stats, err := c.readKstat(arcstatsPath)
	if err != nil {
		// ZFS might not be available, skip silently
		return
	}

	// Map of metric names to prometheus descriptors
	metricMap := map[string]*prometheus.Desc{
		"size":                  c.arcSize,
		"c_max":                 c.arcCMax,
		"c_min":                 c.arcCMin,
		"c":                     c.arcC,
		"p":                     c.arcP,
		"hits":                  c.arcHits,
		"misses":                c.arcMisses,
		"target_size":           c.arcTargetSize,
		"mru_size":              c.arcMRUSize,
		"mfu_size":              c.arcMFUSize,
		"mru_ghost_size":        c.arcMRUGhostSize,
		"mfu_ghost_size":        c.arcMFUGhostSize,
		"data_size":             c.arcDataSize,
		"metadata_size":         c.arcMetadataSize,
		"evict_skip":            c.arcEvictSkip,
		"memory_throttle_count": c.arcMemoryThrottle,
	}

	for name, desc := range metricMap {
		if value, ok := stats[name]; ok {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				value,
			)
		}
	}
}

// collectPoolStats reads and exports pool statistics
func (c *ZFSCollector) collectPoolStats(ch chan<- prometheus.Metric) {
	// Note: Pool stats would typically come from `zpool list` command
	// For now, we'll implement a basic version that reads from zpool command
	// In production, you might want to use a ZFS library or parse zpool output
}

// readKstat reads a kstat file and returns a map of metric name to value
func (c *ZFSCollector) readKstat(path string) (map[string]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stats := make(map[string]float64)
	scanner := bufio.NewScanner(file)

	// Skip header lines
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue // Skip first two header lines
		}

		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		valueStr := fields[2]

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		stats[name] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading kstat file: %w", err)
	}

	return stats, nil
}

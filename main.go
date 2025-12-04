package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bigtcze/pve-exporter/collector"
	"github.com/bigtcze/pve-exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// CLI flags
	showVersion := flag.Bool("version", false, "Print version and exit")
	selfUpdate := flag.Bool("selfupdate", false, "Update to latest version and restart")
	flag.Parse()

	// Handle --version
	if *showVersion {
		fmt.Printf("pve-exporter version=%s commit=%s date=%s\n", version, commit, date)
		os.Exit(0)
	}

	// Handle --selfupdate
	if *selfUpdate {
		if err := SelfUpdate(version); err != nil {
			log.Fatalf("Self-update failed: %v", err)
		}
		os.Exit(0)
	}

	log.Printf("Starting pve-exporter version=%s commit=%s date=%s", version, commit, date)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Connecting to Proxmox at %s:%d", cfg.Proxmox.Host, cfg.Proxmox.Port)

	// Create Prometheus registry
	registry := prometheus.NewRegistry()

	// Register Proxmox collector
	proxmoxCollector := collector.NewProxmoxCollector(&cfg.Proxmox)
	registry.MustRegister(proxmoxCollector)

	// Setup HTTP server
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.Handle(cfg.Server.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog:      log.New(os.Stderr, "", log.LstdFlags),
		ErrorHandling: promhttp.ContinueOnError,
	}))

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	// Root endpoint with info
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html>
<head><title>Proxmox Exporter</title></head>
<body>
<h1>Proxmox Exporter</h1>
<p>Version: %s</p>
<p>Commit: %s</p>
<p>Build Date: %s</p>
<p><a href="%s">Metrics</a></p>
<p><a href="/health">Health</a></p>
</body>
</html>`, version, commit, date, cfg.Server.MetricsPath)
	})

	// Start HTTP server
	server := &http.Server{
		Addr:    cfg.Server.ListenAddress,
		Handler: mux,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		server.Close()
	}()

	log.Printf("Starting HTTP server on %s", cfg.Server.ListenAddress)
	log.Printf("Metrics available at %s", cfg.Server.MetricsPath)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}

	log.Println("Exporter stopped")
}

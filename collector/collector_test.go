package collector

import (
	"testing"

	"github.com/bigtcze/pve-exporter/config"
	"github.com/prometheus/client_golang/prometheus"
)

func TestNewProxmoxCollector(t *testing.T) {
	cfg := &config.ProxmoxConfig{
		Host: "localhost",
		User: "root@pam",
	}

	c := NewProxmoxCollector(cfg)
	if c == nil {
		t.Fatal("NewProxmoxCollector returned nil")
	}
}

func TestDescribe(t *testing.T) {
	cfg := &config.ProxmoxConfig{
		Host: "localhost",
		User: "root@pam",
	}

	c := NewProxmoxCollector(cfg)
	ch := make(chan *prometheus.Desc)

	go func() {
		for range ch {
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Describe panicked: %v", r)
		}
	}()

	c.Describe(ch)
	close(ch)
}

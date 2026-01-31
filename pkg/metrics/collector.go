package metrics

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/power-edge/power-edge/pkg/config"
)

// Collector collects and exposes Prometheus metrics
type Collector struct {
	state   *config.State
	metrics map[string]MetricValue
}

// MetricValue represents a single metric
type MetricValue struct {
	Value       float64
	Labels      map[string]string
	Description string
}

// NewCollector creates a new metrics collector
func NewCollector(state *config.State) *Collector {
	return &Collector{
		state:   state,
		metrics: make(map[string]MetricValue),
	}
}

// CheckAndUpdate runs state checks and updates metrics
func (c *Collector) CheckAndUpdate(state *config.State) error {
	log.Println("Checking services...")
	if err := c.checkServices(state.Services); err != nil {
		log.Printf("Service check error: %v", err)
	}

	log.Println("Checking sysctl parameters...")
	if err := c.checkSysctl(state.Sysctl); err != nil {
		log.Printf("Sysctl check error: %v", err)
	}

	return nil
}

func (c *Collector) checkServices(services []config.ServiceConfig) error {
	for _, svc := range services {
		// Check if service is active
		cmd := exec.Command("systemctl", "is-active", svc.Name)
		output, err := cmd.Output()
		status := strings.TrimSpace(string(output))

		compliant := 0.0
		if err == nil && status == "active" && svc.State == "running" {
			compliant = 1.0
			log.Printf("  ✓ %s: active (compliant)", svc.Name)
		} else {
			log.Printf("  ✗ %s: %s (expected: %s)", svc.Name, status, svc.State)
		}

		c.metrics[fmt.Sprintf("service_compliant{name=%q}", svc.Name)] = MetricValue{
			Value: compliant,
			Labels: map[string]string{
				"name":     svc.Name,
				"expected": string(svc.State),
				"actual":   status,
			},
			Description: "Service compliance (1 = compliant, 0 = non-compliant)",
		}
	}

	return nil
}

func (c *Collector) checkSysctl(params map[string]string) error {
	for key, expectedValue := range params {
		cmd := exec.Command("sysctl", "-n", key)
		output, err := cmd.Output()
		actualValue := strings.TrimSpace(string(output))

		compliant := 0.0
		if err == nil && actualValue == expectedValue {
			compliant = 1.0
			log.Printf("  ✓ %s: %s (compliant)", key, actualValue)
		} else {
			log.Printf("  ✗ %s: %s (expected: %s)", key, actualValue, expectedValue)
		}

		c.metrics[fmt.Sprintf("sysctl_compliant{key=%q}", key)] = MetricValue{
			Value: compliant,
			Labels: map[string]string{
				"key":      key,
				"expected": expectedValue,
				"actual":   actualValue,
			},
			Description: "Sysctl parameter compliance (1 = compliant, 0 = non-compliant)",
		}
	}

	return nil
}

// Handler returns an HTTP handler for Prometheus metrics
func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		// Write metrics in Prometheus format
		for name, metric := range c.metrics {
			// Write HELP
			fmt.Fprintf(w, "# HELP %s\n", metric.Description)
			// Write TYPE
			fmt.Fprintf(w, "# TYPE edge_state_compliance gauge\n")
			// Write metric
			fmt.Fprintf(w, "edge_state_compliance%s %v\n", name, metric.Value)
		}

		// Write metadata
		fmt.Fprintf(w, "# HELP edge_state_info Edge state information\n")
		fmt.Fprintf(w, "# TYPE edge_state_info gauge\n")
		fmt.Fprintf(w, "edge_state_info{site=%q,environment=%q} 1\n",
			c.state.Metadata.Site,
			c.state.Metadata.Environment,
		)
	})
}

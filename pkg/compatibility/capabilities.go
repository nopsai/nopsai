package compatibility

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	CapabilityAPIV1              = "api.v1"
	CapabilityConfigSyncV1       = "config-sync.v1"
	CapabilityDashboardRefreshV1 = "dashboard-refresh.v1"
	CapabilityDashboardsV1       = "dashboards.v1"
	CapabilityMCPV1              = "mcp.v1"
	CapabilityMonitoringV1       = "monitoring.v1"
	CapabilityPlatformHelm       = "platform.helm"
	CapabilityRunnerDocker       = "runner.docker"
	CapabilityRunnerK8s          = "runner.kubernetes"
	CapabilityCLICatalogV1       = "cli.api-catalog.v1"
)

var capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z0-9][a-z0-9-]*)+$`)

func ValidateCapabilities(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	capabilities := make([]string, 0, len(values))
	for _, capability := range values {
		capability = strings.TrimSpace(capability)
		if !capabilityPattern.MatchString(capability) {
			return nil, fmt.Errorf("invalid capability %q", capability)
		}
		if _, exists := seen[capability]; exists {
			return nil, fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	return capabilities, nil
}

func HasCapability(values []string, wanted string) bool {
	for _, capability := range values {
		if capability == wanted {
			return true
		}
	}
	return false
}

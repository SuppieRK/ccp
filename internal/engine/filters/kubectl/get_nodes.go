package kubectlfilters

import "go-command-compression-proxy/internal/engine"

// NewKubectlGetNodesFilter compacts kubectl get nodes output.
func NewKubectlGetNodesFilter() engine.ToolFilter {
	return tableCompactorFilter{cfg: tableCompactorConfig{
		tool:          "kubectl get nodes",
		resourceLabel: "nodes",
		headerOptions: [][]string{
			{"NAME", "STATUS", "ROLES", "AGE", "VERSION"},
		},
		namespaceAware:  false,
		healthEvaluator: isNodesHealthy,
		signature: func(row tableRow) string {
			parts := []string{row.get("STATUS"), row.get("ROLES"), row.get("VERSION")}
			if ip := row.get("INTERNAL-IP"); ip != "" {
				parts = append(parts, "ip="+ip)
			}
			return formatResourceSignature("nodes", parts)
		},
	}}
}

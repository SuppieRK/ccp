package kubectlfilters

import "go-command-compression-proxy/internal/engine"

// NewKubectlGetPodsFilter compacts kubectl get pods output.
func NewKubectlGetPodsFilter() engine.ToolFilter {
	return tableCompactorFilter{cfg: tableCompactorConfig{
		tool:          "kubectl get pods",
		resourceLabel: "pods",
		headerOptions: [][]string{
			{"NAME", "READY", "STATUS", "RESTARTS", "AGE"},
			{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "AGE"},
		},
		namespaceAware:  true,
		healthEvaluator: isPodsHealthy,
		signature: func(row tableRow) string {
			parts := []string{row.get("READY"), row.get("STATUS"), row.get("RESTARTS")}
			if node := row.get("NODE"); node != "" {
				parts = append(parts, "node="+node)
			}
			return formatResourceSignature("pods", parts)
		},
	}}
}

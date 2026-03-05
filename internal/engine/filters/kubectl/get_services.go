package kubectlfilters

import "go-command-compression-proxy/internal/engine"

const servicesPortsHeader = "PORT(S)"

// NewKubectlGetServicesFilter compacts kubectl get services output.
func NewKubectlGetServicesFilter() engine.ToolFilter {
	return tableCompactorFilter{cfg: tableCompactorConfig{
		tool:          "kubectl get services",
		resourceLabel: "services",
		headerOptions: [][]string{
			{"NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", servicesPortsHeader, "AGE"},
			{"NAMESPACE", "NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", servicesPortsHeader, "AGE"},
		},
		namespaceAware: true,
		healthEvaluator: func(row tableRow) (bool, string) {
			return isServicesHealthy()
		},
		signature: func(row tableRow) string {
			return formatResourceSignature("services", []string{row.get("TYPE"), row.get(servicesPortsHeader)})
		},
	}}
}

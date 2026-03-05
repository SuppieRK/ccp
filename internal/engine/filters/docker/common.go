package dockerfilters

import (
	"regexp"
	"sort"
	"strings"
)

const defaultMaxRows = 15

var columnSplit = regexp.MustCompile(`\s{2,}`)
var requiredPSHeaders = []string{"CONTAINER ID", "IMAGE", "STATUS", "NAMES"}
var requiredImagesHeaders = []string{"REPOSITORY", "TAG", "SIZE"}

type psRow struct {
	id              string
	image           string
	status          string
	ports           string
	name            string
	statusIsHealthy bool
	portFoldKey     string
}

func splitColumns(line string) []string {
	parts := columnSplit.Split(strings.TrimSpace(line), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isPSHeader(headers []string) bool {
	return hasRequiredHeaders(headers, requiredPSHeaders)
}

func isImagesHeader(headers []string) bool {
	return hasRequiredHeaders(headers, requiredImagesHeaders)
}

func hasRequiredHeaders(headers []string, required []string) bool {
	normalized := make(map[string]struct{}, len(headers))
	for _, h := range headers {
		key := strings.ToUpper(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		normalized[key] = struct{}{}
	}
	for _, need := range required {
		needKey := strings.ToUpper(strings.TrimSpace(need))
		if _, ok := normalized[needKey]; !ok {
			return false
		}
	}
	return true
}

func parsePSRow(headers []string, line string) (psRow, bool) {
	cols := splitColumns(line)
	if len(headers) == 7 && len(cols) == 6 {
		cols = append(cols[:5], append([]string{"-"}, cols[5:]...)...)
	}
	if len(cols) < len(headers) {
		return psRow{}, false
	}
	id := columnValue(headers, cols, "CONTAINER ID")
	image := columnValue(headers, cols, "IMAGE")
	status := columnValue(headers, cols, "STATUS")
	name := columnValue(headers, cols, "NAMES")
	ports := columnValue(headers, cols, "PORTS")
	if strings.TrimSpace(ports) == "" {
		ports = "-"
	}
	if id == "" || image == "" || status == "" || name == "" {
		return psRow{}, false
	}
	status = strings.TrimSpace(strings.TrimSuffix(status, " -"))
	statusLower := strings.ToLower(status)
	return psRow{
		id:              id,
		image:           image,
		status:          status,
		ports:           ports,
		name:            name,
		statusIsHealthy: strings.HasPrefix(statusLower, "up "),
		portFoldKey:     normalizePortsForFold(ports),
	}, true
}

func columnValue(headers []string, cols []string, name string) string {
	for i, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			if i >= len(cols) {
				return ""
			}
			return cols[i]
		}
	}
	return ""
}

func normalizePortsForFold(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return "-"
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "->") {
			lhs, rhs, _ := strings.Cut(p, "->")
			hostPort := strings.TrimSpace(lhs)
			if idx := strings.LastIndex(hostPort, ":"); idx >= 0 && idx+1 < len(hostPort) {
				hostPort = strings.TrimSpace(hostPort[idx+1:])
			}
			p = hostPort + "->" + strings.TrimSpace(rhs)
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func displayPorts(v string) string {
	if v = strings.TrimSpace(v); v == "" {
		return "-"
	}
	return v
}

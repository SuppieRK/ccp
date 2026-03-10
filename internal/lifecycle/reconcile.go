package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-command-compression-proxy/internal/lifecycle/agents"
	"go-command-compression-proxy/internal/version"
)

const integrationStateVersion = 1

var (
	startupMaintenanceVersion  = func() string { return version.Version }
	startupMaintenanceAdapters = agents.DefaultAdapters
	startupMaintenancePrintf   = fmt.Printf
)

// RunStartupMaintenance opportunistically reconciles configured integrations and
// removes known-obsolete project-local init state without requiring an explicit
// lifecycle command.
func RunStartupMaintenance() error {
	scopeRoot, err := initDetectRoot()
	if err != nil {
		return nil
	}
	if err := cleanupLegacyProjectInitState(scopeRoot); err != nil {
		return err
	}
	cfgPath, err := initPath()
	if err != nil {
		return nil
	}
	_, shouldReconcile, err := loadReconcileCandidate(cfgPath)
	if err != nil || !shouldReconcile {
		return err
	}

	release, err := acquireStartupMaintenanceLock(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer release()

	cfg, shouldReconcile, err := loadReconcileCandidate(cfgPath)
	if err != nil || !shouldReconcile {
		return err
	}
	return reconcileConfiguredTools(cfgPath, scopeRoot, cfg)
}

func readInitConfig(path string) (initConfig, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return initConfig{}, false, nil
		}
		return initConfig{}, false, err
	}
	var cfg initConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return initConfig{}, false, err
	}
	return cfg, true, nil
}

func needsIntegrationReconcile(cfg initConfig) bool {
	return strings.TrimSpace(cfg.CCPVersion) != startupMaintenanceVersion() ||
		cfg.IntegrationVersion != integrationStateVersion
}

func loadReconcileCandidate(cfgPath string) (initConfig, bool, error) {
	cfg, exists, err := readInitConfig(cfgPath)
	if err != nil || !exists || len(cfg.Tools) == 0 {
		return initConfig{}, false, err
	}
	if !needsIntegrationReconcile(cfg) {
		return initConfig{}, false, nil
	}
	return cfg, true, nil
}

func reconcileConfiguredTools(cfgPath, scopeRoot string, cfg initConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	adapters := startupMaintenanceAdapters()
	if err := agents.ValidateSelectedTools(cfg.Tools, adapters); err != nil {
		return err
	}
	states, err := applyAdapters(
		agents.Context{ScopeRoot: scopeRoot, HomeDir: homeDir},
		cfg.Tools,
		adapters,
	)
	if err != nil {
		return err
	}
	cfg.State = states
	cfg.CCPVersion = startupMaintenanceVersion()
	cfg.IntegrationVersion = integrationStateVersion
	changed, err := persistInitConfig(cfgPath, cfg)
	if err != nil {
		return err
	}
	if changed {
		_, _ = startupMaintenancePrintf("ccp: reconciled configured agent integrations\n")
	}
	return nil
}

func acquireStartupMaintenanceLock(cfgPath string) (func(), error) {
	lockPath := cfgPath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

func cleanupLegacyProjectInitState(scopeRoot string) error {
	ccpDir := filepath.Join(scopeRoot, ".ccp")
	targets := []string{filepath.Join(ccpDir, "init.json")}
	matches, err := filepath.Glob(filepath.Join(ccpDir, "init.json.bak.*"))
	if err != nil {
		return err
	}
	targets = append(targets, matches...)
	for _, path := range targets {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

package coverage

import (
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

type PackageStat struct {
	Package    string
	Covered    int64
	Statements int64
	Percent    float64
}

type Report struct {
	ModulePath       string
	InternalPrefix   string
	Threshold        float64
	InternalTotal    PackageStat
	InternalPackages []PackageStat
	OtherPackages    []PackageStat
}

type totals struct {
	covered int64
	stmts   int64
}

type blockKey struct {
	file      string
	startLine int
	startCol  int
	endLine   int
	endCol    int
	numStmt   int
}

type blockStat struct {
	pkgIdx  int
	stmts   int64
	covered bool
}

func ParseProfile(r io.Reader, modulePath, internalPrefix string, threshold float64) (Report, error) {
	if strings.TrimSpace(modulePath) == "" {
		return Report{}, fmt.Errorf("module path is required")
	}
	if strings.TrimSpace(internalPrefix) == "" {
		return Report{}, fmt.Errorf("internal prefix is required")
	}

	profiles, err := cover.ParseProfilesFromReader(r)
	if err != nil {
		return Report{}, fmt.Errorf("parse coverprofile: %w", err)
	}
	byPackage := aggregateByPackage(profiles, modulePath)
	return buildReport(byPackage, modulePath, internalPrefix, threshold), nil
}

func aggregateByPackage(profiles []*cover.Profile, modulePath string) map[string]totals {
	byPackage := map[string]totals{}
	pkgIndex := map[string]int{}
	pkgList := make([]string, 0, 128)
	byBlock := map[blockKey]blockStat{}

	for _, prof := range profiles {
		pkg := packageForFile(prof.FileName, modulePath)
		if pkg == "" {
			continue
		}
		idx, ok := pkgIndex[pkg]
		if !ok {
			idx = len(pkgList)
			pkgIndex[pkg] = idx
			pkgList = append(pkgList, pkg)
		}
		mergeProfileBlocks(byBlock, prof, idx)
	}

	for _, b := range byBlock {
		pkg := pkgList[b.pkgIdx]
		t := byPackage[pkg]
		t.stmts += b.stmts
		if b.covered {
			t.covered += b.stmts
		}
		byPackage[pkg] = t
	}
	return byPackage
}

func mergeProfileBlocks(byBlock map[blockKey]blockStat, prof *cover.Profile, pkgIdx int) {

	for _, block := range prof.Blocks {
		key := blockKey{
			file:      prof.FileName,
			startLine: block.StartLine,
			startCol:  block.StartCol,
			endLine:   block.EndLine,
			endCol:    block.EndCol,
			numStmt:   block.NumStmt,
		}
		agg, exists := byBlock[key]
		if !exists {
			byBlock[key] = blockStat{
				pkgIdx:  pkgIdx,
				stmts:   int64(block.NumStmt),
				covered: block.Count > 0,
			}
			continue
		}
		agg.covered = agg.covered || block.Count > 0
		byBlock[key] = agg
	}
}

func buildReport(byPackage map[string]totals, modulePath, internalPrefix string, threshold float64) Report {
	report := Report{
		ModulePath:     modulePath,
		InternalPrefix: internalPrefix,
		Threshold:      threshold,
	}
	for pkg, t := range byPackage {
		stat := PackageStat{
			Package:    pkg,
			Covered:    t.covered,
			Statements: t.stmts,
			Percent:    percent(t.covered, t.stmts),
		}
		if strings.HasPrefix(pkg, internalPrefix) {
			report.InternalPackages = append(report.InternalPackages, stat)
			report.InternalTotal.Covered += stat.Covered
			report.InternalTotal.Statements += stat.Statements
		} else {
			report.OtherPackages = append(report.OtherPackages, stat)
		}
	}
	sort.Slice(report.InternalPackages, func(i, j int) bool { return report.InternalPackages[i].Package < report.InternalPackages[j].Package })
	sort.Slice(report.OtherPackages, func(i, j int) bool { return report.OtherPackages[i].Package < report.OtherPackages[j].Package })
	report.InternalTotal.Package = internalPrefix
	report.InternalTotal.Percent = percent(report.InternalTotal.Covered, report.InternalTotal.Statements)
	return report
}

func packageForFile(filePath, modulePath string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/")
	normalized = path.Clean(normalized)
	normalized = strings.TrimPrefix(normalized, modulePath+"/")
	dir := path.Dir(normalized)
	if dir == "." || dir == "/" {
		return ""
	}
	return dir
}

func percent(covered, stmts int64) float64 {
	if stmts <= 0 {
		return 0
	}
	p := (float64(covered) * 100.0) / float64(stmts)
	return math.Round(p*100) / 100
}

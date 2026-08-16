package coverage

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/cover"
)

const (
	coverModePrefix = "mode: "
	malformedLine   = "malformed line %q"
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
		return Report{}, errors.New("module path is required")
	}
	if strings.TrimSpace(internalPrefix) == "" {
		return Report{}, errors.New("internal prefix is required")
	}

	profiles, err := parseProfiles(r)
	if err != nil {
		return Report{}, fmt.Errorf("parse coverprofile: %w", err)
	}
	byPackage := aggregateByPackage(profiles, modulePath)
	return buildReport(byPackage, modulePath, internalPrefix, threshold), nil
}

func parseProfiles(r io.Reader) ([]*cover.Profile, error) {
	reader := bufio.NewReader(r)
	mode, err := readProfileMode(reader)
	if err != nil {
		return nil, err
	}

	byFile := make(map[string]*cover.Profile, 128)
	files := make([]string, 0, 128)
	for lineNo := 2; ; lineNo++ {
		line, readErr := readProfileLine(reader)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, readErr
		}
		if line != "" {
			if err := appendProfileBlock(byFile, &files, mode, line); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	profiles := make([]*cover.Profile, 0, len(files))
	for _, fileName := range files {
		profiles = append(profiles, byFile[fileName])
	}
	return profiles, nil
}

func readProfileLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func appendProfileBlock(byFile map[string]*cover.Profile, files *[]string, mode string, line string) error {
	fileName, block, err := parseProfileBlock(line)
	if err != nil {
		return err
	}
	profile, ok := byFile[fileName]
	if !ok {
		profile = &cover.Profile{
			FileName: fileName,
			Mode:     mode,
			Blocks:   make([]cover.ProfileBlock, 0, 16),
		}
		byFile[fileName] = profile
		*files = append(*files, fileName)
	}
	profile.Blocks = append(profile.Blocks, block)
	return nil
}

func readProfileMode(reader *bufio.Reader) (string, error) {
	line, err := readProfileLine(reader)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	mode, ok := strings.CutPrefix(line, coverModePrefix)
	if !ok {
		return "", fmt.Errorf("first line must start with %q", coverModePrefix)
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "", errors.New("coverage mode is required")
	}
	return mode, nil
}

func parseProfileBlock(line string) (string, cover.ProfileBlock, error) {
	countSep := strings.LastIndexByte(line, ' ')
	if countSep <= 0 {
		return "", cover.ProfileBlock{}, fmt.Errorf(malformedLine, line)
	}
	numStmtSep := strings.LastIndexByte(line[:countSep], ' ')
	if numStmtSep <= 0 {
		return "", cover.ProfileBlock{}, fmt.Errorf(malformedLine, line)
	}

	count, err := strconv.Atoi(line[countSep+1:])
	if err != nil {
		return "", cover.ProfileBlock{}, err
	}
	numStmt, err := strconv.Atoi(line[numStmtSep+1 : countSep])
	if err != nil {
		return "", cover.ProfileBlock{}, err
	}

	location := line[:numStmtSep]
	fileSep := strings.LastIndexByte(location, ':')
	if fileSep <= 0 || fileSep == len(location)-1 {
		return "", cover.ProfileBlock{}, fmt.Errorf(malformedLine, line)
	}
	fileName := location[:fileSep]
	start, end, err := parseProfileRange(location[fileSep+1:])
	if err != nil {
		return "", cover.ProfileBlock{}, err
	}

	return fileName, cover.ProfileBlock{
		StartLine: start.line,
		StartCol:  start.col,
		EndLine:   end.line,
		EndCol:    end.col,
		NumStmt:   numStmt,
		Count:     count,
	}, nil
}

type profilePoint struct {
	line int
	col  int
}

func parseProfileRange(raw string) (profilePoint, profilePoint, error) {
	comma := strings.IndexByte(raw, ',')
	if comma <= 0 || comma == len(raw)-1 {
		return profilePoint{}, profilePoint{}, fmt.Errorf("malformed position %q", raw)
	}
	start, err := parseProfilePoint(raw[:comma])
	if err != nil {
		return profilePoint{}, profilePoint{}, err
	}
	end, err := parseProfilePoint(raw[comma+1:])
	if err != nil {
		return profilePoint{}, profilePoint{}, err
	}
	return start, end, nil
}

func parseProfilePoint(raw string) (profilePoint, error) {
	dot := strings.IndexByte(raw, '.')
	if dot <= 0 || dot == len(raw)-1 {
		return profilePoint{}, fmt.Errorf("malformed position %q", raw)
	}
	line, err := strconv.Atoi(raw[:dot])
	if err != nil {
		return profilePoint{}, err
	}
	col, err := strconv.Atoi(raw[dot+1:])
	if err != nil {
		return profilePoint{}, err
	}
	return profilePoint{line: line, col: col}, nil
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
	slices.SortFunc(report.InternalPackages, func(left, right PackageStat) int {
		return cmp.Compare(left.Package, right.Package)
	})
	slices.SortFunc(report.OtherPackages, func(left, right PackageStat) int {
		return cmp.Compare(left.Package, right.Package)
	})
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

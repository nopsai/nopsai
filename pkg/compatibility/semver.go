package compatibility

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease []string
	Build      []string
}

func ParseVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
	if raw == "" {
		return Version{}, errors.New("version is empty")
	}
	coreAndBuild := strings.SplitN(raw, "+", 2)
	coreAndPrerelease := strings.SplitN(coreAndBuild[0], "-", 2)
	parts := strings.Split(coreAndPrerelease[0], ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q must use major.minor.patch", raw)
	}
	numbers := make([]int, 3)
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return Version{}, fmt.Errorf("version %q has an invalid numeric component", raw)
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return Version{}, fmt.Errorf("version %q has an invalid numeric component", raw)
		}
		numbers[index] = value
	}
	version := Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if len(coreAndPrerelease) == 2 {
		identifiers, err := parseIdentifiers(coreAndPrerelease[1], true)
		if err != nil {
			return Version{}, fmt.Errorf("version %q: %w", raw, err)
		}
		version.Prerelease = identifiers
	}
	if len(coreAndBuild) == 2 {
		identifiers, err := parseIdentifiers(coreAndBuild[1], false)
		if err != nil {
			return Version{}, fmt.Errorf("version %q: %w", raw, err)
		}
		version.Build = identifiers
	}
	return version, nil
}

func (v Version) String() string {
	value := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Prerelease) > 0 {
		value += "-" + strings.Join(v.Prerelease, ".")
	}
	if len(v.Build) > 0 {
		value += "+" + strings.Join(v.Build, ".")
	}
	return value
}

func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(v.Prerelease) == 0 && len(other.Prerelease) == 0 {
		return 0
	}
	if len(v.Prerelease) == 0 {
		return 1
	}
	if len(other.Prerelease) == 0 {
		return -1
	}
	limit := min(len(v.Prerelease), len(other.Prerelease))
	for index := 0; index < limit; index++ {
		comparison := compareIdentifier(v.Prerelease[index], other.Prerelease[index])
		if comparison != 0 {
			return comparison
		}
	}
	if len(v.Prerelease) < len(other.Prerelease) {
		return -1
	}
	if len(v.Prerelease) > len(other.Prerelease) {
		return 1
	}
	return 0
}

type Comparator struct {
	Operator string
	Version  Version
}

type Range struct {
	Raw         string
	Comparators []Comparator
}

func ParseRange(raw string) (Range, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), ",", " ")
	if normalized == "" {
		return Range{}, errors.New("version range is empty")
	}
	result := Range{Raw: strings.TrimSpace(raw)}
	for _, token := range strings.Fields(normalized) {
		operator := "="
		versionText := token
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(token, candidate) {
				operator = candidate
				versionText = strings.TrimSpace(strings.TrimPrefix(token, candidate))
				break
			}
		}
		version, err := ParseVersion(versionText)
		if err != nil {
			return Range{}, fmt.Errorf("invalid range %q: %w", raw, err)
		}
		result.Comparators = append(result.Comparators, Comparator{Operator: operator, Version: version})
	}
	return result, nil
}

func (r Range) Contains(version Version) bool {
	for _, comparator := range r.Comparators {
		comparison := version.Compare(comparator.Version)
		matched := map[string]bool{"=": comparison == 0, ">": comparison > 0, ">=": comparison >= 0, "<": comparison < 0, "<=": comparison <= 0}[comparator.Operator]
		if !matched {
			return false
		}
	}
	return len(r.Comparators) > 0
}

func parseIdentifiers(raw string, prerelease bool) ([]string, error) {
	identifiers := strings.Split(raw, ".")
	for _, identifier := range identifiers {
		if identifier == "" {
			return nil, errors.New("version identifier cannot be empty")
		}
		for _, character := range identifier {
			if (character < '0' || character > '9') && (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && character != '-' {
				return nil, fmt.Errorf("invalid version identifier %q", identifier)
			}
		}
		if prerelease && numeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return nil, fmt.Errorf("numeric prerelease identifier %q has a leading zero", identifier)
		}
	}
	return identifiers, nil
}

func compareIdentifier(left, right string) int {
	leftNumeric, rightNumeric := numeric(left), numeric(right)
	if leftNumeric && rightNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
		return strings.Compare(left, right)
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	return strings.Compare(left, right)
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

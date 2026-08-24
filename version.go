// Package nopsai carries the single value every NopsAI release is numbered
// from.
//
// It lives at the repository root, beside version.txt, for two reasons. The
// first is discoverability: cutting a release is a thing a developer does
// deliberately, and it should be obvious where to do it. The second is a Go
// constraint — go:embed cannot reach outside its own package directory, so the
// only way to embed the file without a code generation step is for the package
// to sit where the file sits.
//
// version.txt holds the exact version, not a series with the patch computed
// from the commit count. An explicit number is one a human chooses and can
// reason about: it does not move when history is rewritten, it does not drift
// between a branch and main, and a release can be cut twice from different
// commits without silently changing number. Forgetting to bump it is caught by
// the release pipeline, which refuses to publish when the tag already exists on
// a different commit.
//
// Everything downstream is derived from here. Nothing else in the repository
// should spell a version out by hand.
package nopsai

import (
	_ "embed"
	"strconv"
	"strings"
)

//go:embed version.txt
var version string

// Version is the exact version this checkout releases as, for example
// "0.22.840". Image tags, the Helm chart version and the CLI all carry it,
// because NopsAI releases every component at one version.
func Version() string {
	return strings.TrimSpace(version)
}

// Series is the major.minor part of Version, for example "0.22". It is what
// compatibility is expressed against: a patch release never changes what an
// artifact is compatible with.
func Series() string {
	current := Version()
	major, rest, found := strings.Cut(current, ".")
	if !found {
		return current
	}
	minor, _, _ := strings.Cut(rest, ".")
	return major + "." + minor
}

// BaselineVersion is the first version of the current series, for example
// "0.22.0". It is the lower bound of everything the series is compatible with.
func BaselineVersion() string {
	return Series() + ".0"
}

// CompatibilityRange is the version range every artifact in this series
// accepts: from the series baseline up to, but excluding, the next major.
func CompatibilityRange() string {
	return ">=" + BaselineVersion() + " <" + nextMajor() + ".0.0"
}

func nextMajor() string {
	major, _, found := strings.Cut(Series(), ".")
	if !found {
		major = Series()
	}
	number, err := strconv.Atoi(strings.TrimSpace(major))
	if err != nil {
		// An unparseable version is a broken checkout, not a runtime condition
		// to paper over. Returning the raw value makes the range obviously
		// wrong rather than quietly plausible.
		return major
	}
	return strconv.Itoa(number + 1)
}

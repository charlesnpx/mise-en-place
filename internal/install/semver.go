package install

import (
	"fmt"
	"strconv"
	"strings"
)

// checkMinInstaller compares a skill's required CLI version against the
// running CLI. It accepts plain semver "X.Y.Z" with an optional leading "v".
// The literal value "dev" (the default for unreleased builds) is treated as
// "always satisfies" so that local development against the source tree
// works without needing to bump versions.
func checkMinInstaller(required, running string) error {
	if running == "dev" || running == "" {
		return nil
	}
	r, err := parseSemver(required)
	if err != nil {
		return fmt.Errorf("invalid min_installer %q: %w", required, err)
	}
	c, err := parseSemver(running)
	if err != nil {
		return fmt.Errorf("invalid running mise-en-place version %q: %w", running, err)
	}
	if compare(c, r) < 0 {
		return fmt.Errorf("skill requires mise-en-place >= %s but you have %s; upgrade with: brew upgrade mise-en-place",
			required, running)
	}
	return nil
}

type semver struct {
	major, minor, patch int
}

func parseSemver(s string) (semver, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// Drop pre-release / build metadata for the comparison.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, fmt.Errorf("expected MAJOR[.MINOR[.PATCH]], got %q", s)
	}
	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, fmt.Errorf("non-numeric component %q", p)
		}
		switch i {
		case 0:
			v.major = n
		case 1:
			v.minor = n
		case 2:
			v.patch = n
		}
	}
	return v, nil
}

func compare(a, b semver) int {
	switch {
	case a.major != b.major:
		return sign(a.major - b.major)
	case a.minor != b.minor:
		return sign(a.minor - b.minor)
	case a.patch != b.patch:
		return sign(a.patch - b.patch)
	}
	return 0
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

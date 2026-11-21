package sysutil

// System helpers kept from the removed self-updater: version comparison
// and the release predicate other features (plugins, providers, stats)
// compare versions with.

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/yetone/magpie/internal/lazyre"
)

// Site is magpie's home.
const Site = "https://usemagpie.ai"

// Released reports whether v is a release version rather than a build from
// source ("dev", "0bcb2cc-dirty", git describe's "v0.1.0-3-g0bcb2cc").
func Released(v string) bool {
	s := parse(v)
	return s != nil && !describe.MatchString(s.pre) && !strings.Contains(s.pre, "dirty")
}

// describe matches what git describe adds after a tag: commits since, hash.
var describe = lazyre.New(`^\d+-g[0-9a-f]+`)

// Newer reports whether version a comes after b. A pre-release comes before
// the release it leads up to.
func Newer(a, b string) bool {
	x, y := parse(a), parse(b)
	if x == nil || y == nil {
		return false
	}
	for i := range 3 {
		if x.n[i] != y.n[i] {
			return x.n[i] > y.n[i]
		}
	}
	return comparePre(x.pre, y.pre) > 0
}

// comparePre orders two pre-releases the way semver does (§11): no
// pre-release comes after any; otherwise identifier by identifier, numbers
// as numbers ("beta.10" after "beta.9"), a number before a word, and a
// shorter list before a longer one it starts.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		m, errM := strconv.ParseUint(x[i], 10, 64)
		n, errN := strconv.ParseUint(y[i], 10, 64)
		switch {
		case errM == nil && errN == nil:
			if m != n {
				return cmp.Compare(m, n)
			}
		case errM == nil:
			return -1
		case errN == nil:
			return 1
		default:
			if c := strings.Compare(x[i], y[i]); c != 0 {
				return c
			}
		}
	}
	return cmp.Compare(len(x), len(y))
}

type semver struct {
	n   [3]int
	pre string
}

func parse(v string) *semver {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// build metadata ("+sha.5d0c1dc") plays no part in the order
	v, _, _ = strings.Cut(v, "+")
	v, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		s.n[i] = n
	}
	s.pre = pre
	return &s
}

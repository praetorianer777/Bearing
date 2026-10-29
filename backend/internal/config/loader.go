package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// loader reads variables and collects what it could not read, instead of quietly falling back to a default the
// operator did not ask for.
type loader struct {
	problems []string
}

func (l *loader) problem(p string) { l.problems = append(l.problems, p) }

func (l *loader) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (l *loader) list(key string) []string {
	var out []string
	for part := range strings.SplitSeq(l.str(key, ""), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (l *loader) oneOf(key, def string, allowed ...string) string {
	v := l.str(key, def)
	if !slices.Contains(allowed, v) {
		l.problem(fmt.Sprintf("%s is %q; set it to one of %s.", key, v, strings.Join(allowed, ", ")))
		return def
	}
	return v
}

func (l *loader) boolean(key string, def bool) bool {
	v := l.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.problem(fmt.Sprintf("%s is %q; set it to true or false.", key, v))
		return def
	}
	return b
}

func (l *loader) positive(key string, def int) int {
	v := l.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		l.problem(fmt.Sprintf("%s is %q; set it to a whole number above zero, such as %d.", key, v, def))
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v := l.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		l.problem(fmt.Sprintf("%s is %q; set it to a duration such as %s.", key, v, def))
		return def
	}
	return d
}

package fieldpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Parse parses the path syntax of base §4.2.1: dot-separated names, [*] for
// any list element, ["key"] for a key containing dots or slashes, and an
// optional leading **.
//
// It rejects empty paths and steps, unbalanced brackets, bad quoting, and **
// anywhere but the start. Inside quotes a backslash escapes the next
// character as in a Go string literal.
func Parse(s string) (Path, error) {
	if s == "" {
		return Path{}, errors.New("empty path")
	}
	var p Path
	rest := s
	if after, ok := strings.CutPrefix(rest, "**."); ok {
		p.AnyDepth = true
		rest = after
	}
	i := 0
	for first := true; ; first = false {
		// A name starts a step. A bracket may start the path, never a step
		// after a dot.
		if i < len(rest) && rest[i] == '[' {
			if !first {
				return Path{}, fmt.Errorf("%q: empty step before [", s)
			}
		} else {
			name, err := scanName(rest[i:])
			if err != nil {
				return Path{}, fmt.Errorf("%q: %w", s, err)
			}
			p.Steps = append(p.Steps, Step{Name: name})
			i += len(name)
		}
		for i < len(rest) && rest[i] == '[' {
			step, n, err := scanBracket(rest[i:])
			if err != nil {
				return Path{}, fmt.Errorf("%q: %w", s, err)
			}
			p.Steps = append(p.Steps, step)
			i += n
		}
		if i == len(rest) {
			return p, nil
		}
		if rest[i] != '.' {
			return Path{}, fmt.Errorf("%q: unexpected %q", s, rest[i])
		}
		i++
	}
}

// scanName reads a plain name from the start of s, up to the next separator.
func scanName(s string) (string, error) {
	name := s
	if n := strings.IndexAny(s, ".[]\""); n >= 0 {
		name = s[:n]
	}
	switch {
	case name == "":
		return "", errors.New("empty step")
	case name == "**":
		return "", errors.New("** is only allowed at the start")
	case strings.Contains(name, "*"):
		return "", fmt.Errorf("name %q holds *; use [*] for any list element", name)
	}
	return name, nil
}

// scanBracket reads [*] or ["key"] from the start of s and returns the number
// of bytes it used.
func scanBracket(s string) (Step, int, error) {
	if strings.HasPrefix(s, "[*]") {
		return Step{AnyElem: true}, 3, nil
	}
	if !strings.HasPrefix(s, `["`) {
		return Step{}, 0, errors.New(`[ must open [*] or ["key"]`)
	}
	end := -1
	for i := 2; i < len(s) && end < 0; i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			end = i
		}
	}
	if end < 0 {
		return Step{}, 0, errors.New("unterminated quoted key")
	}
	if end+1 >= len(s) || s[end+1] != ']' {
		return Step{}, 0, errors.New(`quoted key must end with "]`)
	}
	key, err := strconv.Unquote(s[1 : end+1])
	if err != nil {
		return Step{}, 0, fmt.Errorf("bad quoted key: %w", err)
	}
	if key == "" {
		return Step{}, 0, errors.New("empty key")
	}
	return Step{Name: key}, end + 2, nil
}

// MustParse is Parse for the compiled-in catalogue; it panics on error.
func MustParse(s string) Path {
	p, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return p
}

// String renders p back in the syntax Parse accepts.
func (p Path) String() string {
	var b strings.Builder
	if p.AnyDepth {
		b.WriteString("**.")
	}
	for i, st := range p.Steps {
		if st.AnyElem {
			b.WriteString("[*]")
			continue
		}
		writeKey(&b, st.Name, i == 0)
	}
	return b.String()
}

// Format renders a concrete path for findings, for example
// spec.template.spec.containers[0].image or metadata.labels["a.b/c"].
func Format(path []Segment) string {
	var b strings.Builder
	for i, seg := range path {
		switch {
		case seg.Elem && seg.Key == "":
			b.WriteString("[*]")
		case seg.Elem:
			// A numeric or "-" segment may be a map key too; it renders as
			// the list element it most likely is.
			b.WriteString("[" + seg.Key + "]")
		default:
			writeKey(&b, seg.Key, i == 0)
		}
	}
	return b.String()
}

// writeKey writes a field or map key: bare after a dot when it is a plain
// name, else as ["key"].
func writeKey(b *strings.Builder, key string, first bool) {
	if isPlainName(key) {
		if !first {
			b.WriteByte('.')
		}
		b.WriteString(key)
		return
	}
	b.WriteString("[" + strconv.Quote(key) + "]")
}

func isPlainName(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return strings.ContainsRune(".[]\"*\\", r) || !unicode.IsPrint(r) || unicode.IsSpace(r)
	})
}

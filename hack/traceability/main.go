// Command traceability checks test/TRACEABILITY.md against the spec's test
// IDs and the repository's tests. The rules are listed at the top of that
// file. Run with `make traceability`.
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const file = "test/TRACEABILITY.md"

// expectedIDs are every acceptance test ID in the specs.
func expectedIDs() []string {
	var ids []string
	add := func(prefix string, n int, skip ...int) {
		for i := 1; i <= n; i++ {
			if !slices.Contains(skip, i) {
				ids = append(ids, fmt.Sprintf("%s%d", prefix, i))
			}
		}
	}
	add("T", 53)
	add("HT", 38)
	add("GT", 42, 38) // GT38 was dropped (GitOps R7)
	add("UT", 31)
	add("HS", 12)
	return ids
}

var (
	idRe       = regexp.MustCompile(`^(?:HT|GT|UT|HS|T)\d+$`)
	testFuncRe = regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	refRe      = regexp.MustCompile(`^([\w./-]+\.go):(Test\w+)$`)
)

type row struct {
	id, tasks, status string
	line              int
}

func main() {
	problems, err := check()
	if err != nil {
		fmt.Fprintln(os.Stderr, "traceability:", err)
		os.Exit(2)
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "traceability:", p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
	fmt.Println("traceability: ok")
}

func check() ([]string, error) {
	rows, done, err := parse(file)
	if err != nil {
		return nil, err
	}
	tests, err := testFuncs(".")
	if err != nil {
		return nil, err
	}

	var problems []string
	byID := map[string]row{}
	for _, r := range rows {
		if _, dup := byID[r.id]; dup {
			problems = append(problems, fmt.Sprintf("%s:%d: %s listed twice", file, r.line, r.id))
		}
		byID[r.id] = r
	}
	for _, id := range expectedIDs() {
		if _, ok := byID[id]; !ok {
			problems = append(problems, fmt.Sprintf("%s missing from %s", id, file))
		}
	}

	for _, r := range rows {
		if !slices.Contains(expectedIDs(), r.id) {
			problems = append(problems, fmt.Sprintf("%s:%d: %s is no spec test ID", file, r.line, r.id))
		}
		refs, perr := refs(r.status)
		if perr != "" {
			problems = append(problems, fmt.Sprintf("%s:%d: %s: %s", file, r.line, r.id, perr))
		}
		for _, ref := range refs {
			if !slices.Contains(tests[ref.file], ref.test) {
				problems = append(problems, fmt.Sprintf("%s:%d: %s names %s:%s, which doesn't exist", file, r.line, r.id, ref.file, ref.test))
			}
		}
		if r.status == "unclaimed" {
			for _, task := range strings.Split(r.tasks, ",") {
				if slices.Contains(done, strings.TrimSpace(task)) {
					problems = append(problems, fmt.Sprintf("%s:%d: %s is unclaimed but its task %s is done", file, r.line, r.id, strings.TrimSpace(task)))
				}
			}
		}
	}

	// Every test that names a spec ID is listed in that ID's row.
	for f, names := range tests {
		for _, name := range names {
			for _, part := range strings.Split(strings.TrimPrefix(name, "Test"), "_") {
				if !idRe.MatchString(part) {
					continue
				}
				r, ok := byID[part]
				if !ok || !strings.Contains(r.status, f+":"+name) {
					problems = append(problems, fmt.Sprintf("%s:%s names %s but isn't listed in its row", f, name, part))
				}
			}
		}
	}
	return problems, nil
}

type ref struct{ file, test string }

// refs parses an "Implemented by" cell.
func refs(status string) ([]ref, string) {
	switch {
	case status == "unclaimed", strings.HasPrefix(status, "dropped ("):
		return nil, ""
	}
	s := strings.TrimPrefix(status, "partial: ")
	if i := strings.Index(s, " ("); i >= 0 {
		if !strings.HasSuffix(s, ")") {
			return nil, "unterminated note"
		}
		s = s[:i]
	}
	var out []ref
	for _, part := range strings.Split(s, ",") {
		m := refRe.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return out, fmt.Sprintf("can't parse %q as path:TestName", strings.TrimSpace(part))
		}
		out = append(out, ref{m[1], m[2]})
	}
	return out, ""
}

func parse(path string) ([]row, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var rows []row
	var done []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, "Tasks done:"); ok {
			for _, t := range strings.Split(rest, ",") {
				done = append(done, strings.TrimSpace(t))
			}
			continue
		}
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), " | ")
		if len(cells) != 6 {
			return nil, nil, fmt.Errorf("%s:%d: want 6 cells, got %d", path, n, len(cells))
		}
		id := strings.TrimSpace(cells[0])
		if !idRe.MatchString(id) {
			continue // header rows
		}
		rows = append(rows, row{id: id, tasks: strings.TrimSpace(cells[3]), status: strings.TrimSpace(cells[5]), line: n})
	}
	return rows, done, sc.Err()
}

// testFuncs maps each _test.go file (slash path) to its test functions.
func testFuncs(root string) (map[string][]string, error) {
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "bin") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range testFuncRe.FindAllStringSubmatch(string(src), -1) {
			out[filepath.ToSlash(path)] = append(out[filepath.ToSlash(path)], m[1])
		}
		return nil
	})
	return out, err
}

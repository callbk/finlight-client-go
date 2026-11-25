package finlight

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// R-DOCS-1: README.md follows the shared skeleton.
func TestReadmeSkeleton(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	inCode := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
		}
		if !inCode && (strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")) {
			got = append(got, line)
		}
	}
	want := []string{
		"# finlight client for Go", "## Versions", "## Requirements", "## Installation", "## Quick start",
		"## Search articles", "## Look up articles, companies and sources", "## Absent, null and empty",
		"## Real-time streams", "## Webhooks", "## Errors and retries", "## Configuration", "## Logging",
		"## Data notes", "## Migrating from v1", "## Development", "## License",
	}
	if !slices.Equal(got, want) {
		t.Errorf("README headings:\n%s", strings.Join(got, "\n"))
	}
	text := string(data)
	for _, must := range []string{"| SDK version | API version | Status |", "> **Breaking release.**", "docs.finlight.me", "never 410", "32,000 bytes"} {
		if !strings.Contains(text, must) {
			t.Errorf("README lacks %q", must)
		}
	}
	// The translations have the same number of headings and code blocks.
	for _, tr := range []string{"README.zh-CN.md", "README.ja.md", "README.ko.md"} {
		td, err := os.ReadFile(tr)
		if err != nil {
			t.Fatal(err)
		}
		if n, m := strings.Count(string(td), "\n## "), strings.Count(text, "\n## "); n != m {
			t.Errorf("%s has %d sections, README.md %d", tr, n, m)
		}
		if n, m := strings.Count(string(td), "```"), strings.Count(text, "```"); n != m {
			t.Errorf("%s has %d code fences, README.md %d", tr, n, m)
		}
		if !strings.Contains(string(td), "github.com/callbk/finlight-client-go/v2") {
			t.Errorf("%s does not use the /v2 import path", tr)
		}
	}
}

// R-TEST-1: CASES.md maps every kit case.
func TestCasesTable(t *testing.T) {
	data, err := os.ReadFile("CASES.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range kitCases(t) {
		if !strings.Contains(string(data), "| "+c.ID+" | ") {
			t.Errorf("CASES.md lacks %s", c.ID)
		}
	}
}

// R-DOCS-1: the README quick start compiles against this module.
func TestReadmeQuickStartCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	data, _ := os.ReadFile("README.md")
	m := regexp.MustCompile("(?s)## Quick start\n\n```go\n(.*?)```").FindSubmatch(data)
	if m == nil {
		t.Fatal("no quick start block")
	}
	root, _ := filepath.Abs(".")
	dir := t.TempDir()
	gomod := "module readmecheck\n\ngo 1.24\n\nrequire github.com/callbk/finlight-client-go/v2 v2.0.0\n\nreplace github.com/callbk/finlight-client-go/v2 => " + root + "\n"
	sum, _ := os.ReadFile("go.sum")
	for name, content := range map[string][]byte{"go.mod": []byte(gomod), "main.go": m[1], "go.sum": sum} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(goBin, "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("module cache not ready: %s", out)
	}
	cmd = exec.Command(goBin, "vet", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("quick start does not compile: %s", out)
	}
}

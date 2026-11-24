package finlight

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The contract kit is vendored under testdata/conformance (see
// internal/tools/synckit). Every case of conformance.md runs as its own
// subtest of TestConformance, named "<id> <title>"; CASES.md maps each case to
// its test function.

const kitDir = "testdata/conformance"

const kitAPIKey = "sk_0000000000000000000000000000000000000000000000000000000000000001"

type kitCase struct {
	ID    string
	Title string
}

var caseLine = regexp.MustCompile(`^\| ([A-Z]-\d\d) \| (.+?) \| `)

func kitCases(t testing.TB) []kitCase {
	t.Helper()
	f, err := os.Open(filepath.Join(kitDir, "conformance.md"))
	if err != nil {
		t.Fatalf("open conformance.md: %v", err)
	}
	defer f.Close()
	var out []kitCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := caseLine.FindStringSubmatch(sc.Text()); m != nil {
			out = append(out, kitCase{ID: m[1], Title: m[2]})
		}
	}
	return out
}

var (
	registryMu sync.Mutex
	registry   = map[string]func(t *testing.T){}
)

// conformance registers the test of one kit case.
func conformance(id string, fn func(t *testing.T)) bool {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[id]; dup {
		panic("duplicate conformance test for " + id)
	}
	registry[id] = fn
	return true
}

// TestConformance runs every case of the kit; a case without a test fails.
// The W cases (webhooks) run in the webhook package, whose own
// TestConformance checks them the same way.
func TestConformance(t *testing.T) {
	cases := kitCases(t)
	if len(cases) != 106 {
		t.Fatalf("conformance.md lists %d cases, want 106", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if strings.HasPrefix(c.ID, "W-") {
			continue
		}
		seen[c.ID] = true
		fn := registry[c.ID]
		t.Run(c.ID+" "+c.Title, func(t *testing.T) {
			if fn == nil {
				t.Fatalf("kit case %s has no test", c.ID)
			}
			fn(t)
		})
	}
	for id := range registry {
		if !seen[id] {
			t.Errorf("test registered for %s, which is not a kit case", id)
		}
	}
}

// TestKitManifest fails when the vendored kit differs from its MANIFEST.
func TestKitManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(kitDir, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			t.Fatalf("bad MANIFEST line %q", line)
		}
		want[parts[1]] = parts[0]
	}
	got := map[string]string{}
	err = filepath.WalkDir(kitDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(kitDir, p)
		rel = filepath.ToSlash(rel)
		if rel == "MANIFEST" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		got[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for p, h := range want {
		if got[p] == "" {
			problems = append(problems, "missing: "+p)
		} else if got[p] != h {
			problems = append(problems, "changed: "+p)
		}
	}
	for p := range got {
		if want[p] == "" {
			problems = append(problems, "not in MANIFEST: "+p)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("vendored kit does not match MANIFEST:\n%s", strings.Join(problems, "\n"))
	}
	v, _ := os.ReadFile(filepath.Join(kitDir, "CONTRACT_VERSION"))
	if strings.TrimSpace(string(v)) != "3.0.0-rc.5" {
		t.Fatalf("CONTRACT_VERSION = %q", v)
	}
}

// loadFixture reads a kit file into a generic JSON tree.
func loadFixture(t testing.TB, rel string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(kitDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	return m
}

// rawFixture reads a kit file keeping the raw JSON of each member.
func rawFixture(t testing.TB, rel string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(kitDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	return m
}

func mustJSON(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// jsonEqual compares two JSON documents structurally.
func jsonEqual(t testing.TB, got, want []byte) bool {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON: %v (%s)", err, got)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	return string(gb) == string(wb)
}

func regexpMatch(pattern, s string) bool { return regexp.MustCompile(pattern).MatchString(s) }

// Command synckit copies the API v3 contract kit into testdata/conformance.
//
//	go run ./internal/tools/synckit <path to shared/contracts/api-v3>
//	go run ./internal/tools/synckit -check
//
// The source kit is verified against its MANIFEST (sha256 per file) before
// anything is copied; the old copy is replaced as a whole. With -check the
// vendored copy is verified against its MANIFEST and nothing is written.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const dest = "testdata/conformance"

func main() {
	check := flag.Bool("check", false, "verify the vendored kit against its MANIFEST")
	flag.Parse()
	if *check {
		exit(verify(dest))
		fmt.Println("testdata/conformance matches its MANIFEST")
		return
	}
	if flag.NArg() != 1 {
		exit(errors.New("usage: synckit <path to shared/contracts/api-v3> | synckit -check"))
	}
	src := flag.Arg(0)
	exit(verify(src))
	exit(os.RemoveAll(dest))
	exit(filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}))
	exit(verify(dest))
	v, _ := os.ReadFile(filepath.Join(dest, "CONTRACT_VERSION"))
	fmt.Printf("synced contract kit %s into %s\n", strings.TrimSpace(string(v)), dest)
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "synckit:", err)
		os.Exit(1)
	}
}

// verify checks every file of dir against dir/MANIFEST, both ways.
func verify(dir string) error {
	f, err := os.Open(filepath.Join(dir, "MANIFEST"))
	if err != nil {
		return err
	}
	defer f.Close()
	want := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) == 2 {
			want[parts[1]] = parts[0]
		}
	}
	var problems []string
	seen := map[string]bool{}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "MANIFEST" {
			return nil
		}
		seen[rel] = true
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		switch h, ok := want[rel]; {
		case !ok:
			problems = append(problems, "not in MANIFEST: "+rel)
		case h != hex.EncodeToString(sum[:]):
			problems = append(problems, "checksum differs: "+rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for p := range want {
		if !seen[p] {
			problems = append(problems, "missing: "+p)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s does not match its MANIFEST:\n%s", dir, strings.Join(problems, "\n"))
	}
	return nil
}

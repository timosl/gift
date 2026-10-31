package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAPIDataIsCurrent fails when docs/gift.dev/data/api.json does not match
// the source, so that the API tables of the site cannot fall behind silently.
func TestAPIDataIsCurrent(t *testing.T) {
	want, err := generate(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../gift.dev/data/api.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("docs/gift.dev/data/api.json is out of date, run: go run ./docs/apigen")
	}
}

// TestTheShortcodesNameWhatTheAPIDataHas fails for an api or api-funcs
// shortcode on a page that names a type or a function api.json does not have,
// which the site build would otherwise only report on the build server. A
// function that returns a type of its package is that type's constructor and
// is listed with the type, not among the functions: SecureField, which
// returns a TextFieldView, was named in api-funcs and broke the build.
func TestTheShortcodesNameWhatTheAPIDataHas(t *testing.T) {
	data, err := os.ReadFile("../gift.dev/data/api.json")
	if err != nil {
		t.Fatal(err)
	}
	var api API
	if err := json.Unmarshal(data, &api); err != nil {
		t.Fatal(err)
	}
	shortcode := regexp.MustCompile(`\{\{<\s*(api-funcs|api)\b(.*?)>\}\}`)
	named := regexp.MustCompile(`(\w+)="([^"]*)"`)
	fence := regexp.MustCompile("(?s)```.*?```")
	err = filepath.WalkDir("../gift.dev/content", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := fence.ReplaceAllString(string(src), "")
		for _, m := range shortcode.FindAllStringSubmatch(text, -1) {
			args := map[string]string{"pkg": "ui"}
			for _, a := range named.FindAllStringSubmatch(m[2], -1) {
				args[a[1]] = a[2]
			}
			rest := strings.TrimSpace(named.ReplaceAllString(m[2], ""))
			pkg, ok := api[args["pkg"]]
			if !ok {
				t.Errorf("%s: %s names the package %q, which api.json does not have", path, m[1], args["pkg"])
				continue
			}
			switch m[1] {
			case "api":
				typ := args["type"]
				if typ == "" {
					typ = strings.Trim(rest, `"`)
				}
				if _, ok := pkg.Types[typ]; !ok {
					t.Errorf("%s: api names the type %s.%s, which api.json does not have", path, args["pkg"], typ)
				}
			case "api-funcs":
				have := map[string]bool{}
				for _, f := range pkg.Funcs {
					have[f.Name] = true
				}
				for _, n := range strings.Split(strings.ReplaceAll(args["names"], " ", ""), ",") {
					if n != "" && !have[n] {
						t.Errorf("%s: api-funcs names %s.%s, which is not a package level function in api.json; "+
							"a constructor is listed with the type it returns, use the api shortcode for that type",
							path, args["pkg"], n)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

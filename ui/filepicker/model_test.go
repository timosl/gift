package filepicker

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var day = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"b.txt":           {Data: []byte("hello"), ModTime: day},
		"A.JPG":           {Data: make([]byte, 2400), ModTime: day},
		"c.png":           {Data: nil, ModTime: day},
		".hidden":         {Data: []byte("x")},
		"Zeta/one.txt":    {Data: []byte("1")},
		"alpha/two.txt":   {Data: []byte("2")},
		"alpha/sub/x.jpg": {Data: []byte("3")},
		"link-to-alpha":   {Data: []byte("alpha"), Mode: fs.ModeSymlink},
		"dangling":        {Data: []byte("nowhere"), Mode: fs.ModeSymlink},
	}
}

func names(l listing) []string {
	out := make([]string, len(l.entries))
	for i, e := range l.entries {
		out[i] = e.name
	}
	return out
}

func find(t *testing.T, l listing, name string) entry {
	t.Helper()
	for _, e := range l.entries {
		if e.name == name {
			return e
		}
	}
	t.Fatalf("%q is not in the listing %v", name, names(l))
	return entry{}
}

func TestAListingPutsDirectoriesFirstAndIgnoresCase(t *testing.T) {
	l, err := listDir(testFS(), ".", rules{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "link-to-alpha", "Zeta", "A.JPG", "b.txt", "c.png", "dangling"}
	if got := names(l); !slices.Equal(got, want) {
		t.Fatalf("listing is %v, want %v", got, want)
	}
}

func TestHiddenEntriesAreShownOnlyWhenAskedFor(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{})
	if slices.Contains(names(l), ".hidden") {
		t.Fatal("a dot file is listed without ShowHidden")
	}
	l, _ = listDir(testFS(), ".", rules{showHidden: true})
	if !slices.Contains(names(l), ".hidden") {
		t.Fatal("a dot file is not listed with ShowHidden")
	}
}

func TestALinkIsWhatItPointsToAndABrokenOneIsListedButCannotBePicked(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{})
	if e := find(t, l, "link-to-alpha"); !e.dir || e.broken {
		t.Errorf("a link to a directory is %+v, want a directory that is not broken", e)
	}
	if e := find(t, l, "dangling"); !e.broken || e.pickable {
		t.Errorf("a dangling link is %+v, want broken and not pickable", e)
	}
}

func TestExtensionsAndTheFilterRuleOutFilesButNeverDirectories(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{exts: normalizeExts([]string{"JPG", ".png "})})
	for name, want := range map[string]bool{"A.JPG": true, "c.png": true, "b.txt": false, "alpha": true} {
		if got := find(t, l, name).pickable; got != want {
			t.Errorf("with extensions jpg and png, %s is pickable=%v, want %v", name, got, want)
		}
	}

	small := func(de fs.DirEntry) bool {
		info, err := de.Info()
		return err == nil && info.Size() < 1000
	}
	l, _ = listDir(testFS(), ".", rules{filter: small})
	if find(t, l, "A.JPG").pickable || !find(t, l, "b.txt").pickable || !find(t, l, "Zeta").pickable {
		t.Error("a filter on size did not rule out exactly the large file")
	}
}

func TestDirectoryModeListsFilesDisabled(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{mode: Directory})
	if find(t, l, "b.txt").pickable {
		t.Error("a file is pickable in Directory mode")
	}
	if !find(t, l, "alpha").pickable {
		t.Error("a directory is not openable in Directory mode")
	}
}

func TestAListingCarriesSizeAndDate(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{})
	if got := describe(find(t, l, "A.JPG")); got != "2.4 KB, 12 Aug 2026" {
		t.Errorf("A.JPG is described as %q", got)
	}
	if got := describe(find(t, l, "alpha")); got != "" {
		t.Errorf("a directory is described as %q, want nothing", got)
	}
}

func TestAMissingDirectoryIsAnError(t *testing.T) {
	if _, err := listDir(testFS(), "nope", rules{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("listing a missing directory returned %v, want fs.ErrNotExist", err)
	}
}

func TestFilterEntriesMatchesASubstringIgnoringCase(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{})
	got := filterEntries(nil, l.entries, " ALP ")
	if len(got) != 2 || got[0].name != "alpha" || got[1].name != "link-to-alpha" {
		t.Fatalf("filtering for alp gave %v", got)
	}
	if got := filterEntries(nil, l.entries, ""); len(got) != len(l.entries) {
		t.Fatalf("an empty filter kept %d of %d entries", len(got), len(l.entries))
	}
}

func TestCrumbsAndParent(t *testing.T) {
	got := crumbs("alpha/sub", "Files")
	want := []crumb{{"Files", "."}, {"alpha", "alpha"}, {"sub", "alpha/sub"}}
	if !slices.Equal(got, want) {
		t.Fatalf("crumbs are %v, want %v", got, want)
	}
	if got := crumbs(".", "Files"); len(got) != 1 {
		t.Fatalf("the root has crumbs %v", got)
	}
	if parent("alpha/sub") != "alpha" || parent("alpha") != "." || parent(".") != "." {
		t.Fatal("parent does not walk up to the root and stop there")
	}
}

func TestValidateName(t *testing.T) {
	for name, want := range map[string]error{
		"photo.jpg": nil,
		"":          errNameEmpty,
		"   ":       errNameEmpty,
		"a/b":       errNameSlash,
		".":         errNameDots,
		"..":        errNameDots,
	} {
		if got := validateName("alpha", name); got != want {
			t.Errorf("validateName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0 bytes", 1: "1 byte", 999: "999 bytes", 1000: "1.0 KB",
		2400: "2.4 KB", 15_000_000: "15 MB", 3_200_000_000: "3.2 GB",
	} {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
	if !strings.HasSuffix(formatSize(1<<62), "PB") {
		t.Error("a huge size ran out of units")
	}
}

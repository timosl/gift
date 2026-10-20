package filepicker

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Mode is what the picker picks.
type Mode uint8

const (
	// Open picks one existing file. A tap selects a file and a second tap
	// on the selected file picks it.
	Open Mode = iota
	// OpenMulti picks one or more existing files. A tap toggles a file.
	OpenMulti
	// Directory picks a directory: the one the picker is showing when the
	// primary action is taken. Files are shown, disabled, so that the
	// person can see what a directory contains.
	Directory
	// Save picks a path for a new file: the directory the picker is showing
	// and a name typed into its field. The file may exist; the picker then
	// asks before it reports the path. The picker never writes anything.
	Save
)

// entry is one item of a directory listing, already resolved: a symbolic link
// is reported as what it points to, and as broken when that cannot be read.
type entry struct {
	name    string
	dir     bool
	size    int64
	modTime time.Time
	// broken is a link that does not resolve, or an entry whose information
	// could not be read. It is listed, so that the person can see it is
	// there, and cannot be opened or picked.
	broken bool
	// pickable is false for an entry the mode or the filter rules out; it is
	// listed disabled. A directory is always pickable in the sense that it
	// can be opened.
	pickable bool
}

// listing is what [listDir] returns: the entries of one directory, in the
// order they are shown.
type listing struct {
	dir     string
	entries []entry
	// index maps every name to whether it is a directory, so that a name
	// typed in Save mode is checked without walking the entries on every
	// keystroke. It is built with the listing, off the UI goroutine.
	index map[string]bool
}

// rules is the part of the picker's declaration the listing depends on.
type rules struct {
	mode       Mode
	showHidden bool
	exts       []string // lower case, with the dot
	filter     func(fs.DirEntry) bool
}

// listDir reads dir from fsys and returns its entries under r: hidden entries
// dropped unless asked for, symbolic links resolved, files the rules rule out
// marked, and everything sorted directories first and then by name, ignoring
// case.
//
// It does all of its I/O here, so that the picker can run it off the UI
// goroutine and hand the result over in one piece.
func listDir(fsys fs.FS, dir string, r rules) (listing, error) {
	des, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return listing{}, err
	}
	out := make([]entry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if !r.showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		e := entry{name: name, dir: de.IsDir()}
		var info fs.FileInfo
		if de.Type()&fs.ModeSymlink != 0 {
			// What the link points to, which is what opening it would open.
			info, err = fs.Stat(fsys, path.Join(dir, name))
		} else {
			info, err = de.Info()
		}
		if err != nil {
			e.broken = true
		} else {
			e.dir = info.IsDir()
			e.size = info.Size()
			e.modTime = info.ModTime()
		}
		e.pickable = !e.broken && (e.dir || r.fileAllowed(de))
		out = append(out, e)
	}
	slices.SortFunc(out, compareEntries)
	index := make(map[string]bool, len(out))
	for _, e := range out {
		index[e.name] = e.dir
	}
	return listing{dir: dir, entries: out, index: index}, nil
}

// fileAllowed reports whether a file may be picked under r.
func (r rules) fileAllowed(de fs.DirEntry) bool {
	if r.mode == Directory {
		return false
	}
	if len(r.exts) > 0 && !slices.Contains(r.exts, strings.ToLower(path.Ext(de.Name()))) {
		return false
	}
	return r.filter == nil || r.filter(de)
}

// compareEntries orders directories before files and then by name, ignoring
// case, with the exact name as the tie break so that the order is total.
func compareEntries(a, b entry) int {
	if a.dir != b.dir {
		if a.dir {
			return -1
		}
		return 1
	}
	if c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); c != 0 {
		return c
	}
	return strings.Compare(a.name, b.name)
}

// normalizeExts turns ".JPG", "jpg" and "jpg " into ".jpg".
func normalizeExts(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out = append(out, e)
	}
	return out
}

// filterEntries returns the entries whose name contains q, ignoring case, or
// all of them for an empty q. It reuses dst.
func filterEntries(dst, all []entry, q string) []entry {
	dst = dst[:0]
	q = strings.ToLower(strings.TrimSpace(q))
	for _, e := range all {
		if q == "" || strings.Contains(strings.ToLower(e.name), q) {
			dst = append(dst, e)
		}
	}
	return dst
}

// parent is the directory above dir, and dir itself for the root.
func parent(dir string) string { return path.Dir(dir) }

// crumb is one segment of the path bar.
type crumb struct {
	label string
	dir   string
}

// crumbs splits dir into the segments of the path bar, the root first under
// rootLabel.
func crumbs(dir, rootLabel string) []crumb {
	out := []crumb{{label: rootLabel, dir: "."}}
	if dir == "." {
		return out
	}
	acc := ""
	for _, part := range strings.Split(dir, "/") {
		acc = path.Join(acc, part)
		out = append(out, crumb{label: part, dir: acc})
	}
	return out
}

// Errors of a name typed in Save mode.
var (
	errNameEmpty    = errors.New("a name is needed")
	errNameSlash    = errors.New("a name cannot contain /")
	errNameDots     = errors.New(`"." and ".." are not names`)
	errNameInvalid  = errors.New("that is not a valid name")
	errNameIsFolder = errors.New("a folder of that name exists")
)

// validateName checks a name typed in Save mode, for a file in dir.
func validateName(dir, name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errNameEmpty
	case strings.ContainsRune(name, '/'):
		return errNameSlash
	case name == "." || name == "..":
		return errNameDots
	case !fs.ValidPath(path.Join(dir, name)):
		return errNameInvalid
	}
	return nil
}

// describe is the subtitle of an entry: size and date for a file, nothing for
// a directory, and the reason for a broken entry.
func describe(e entry) string {
	switch {
	case e.broken:
		return "cannot be read"
	case e.dir:
		return ""
	}
	s := formatSize(e.size)
	if !e.modTime.IsZero() {
		s += ", " + e.modTime.Format("2 Jan 2006")
	}
	return s
}

// formatSize writes n bytes in decimal units, as file managers do: 512 bytes,
// 4 KB, 2.4 MB.
func formatSize(n int64) string {
	if n < 1000 {
		if n == 1 {
			return "1 byte"
		}
		return strconv.FormatInt(n, 10) + " bytes"
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	v := float64(n)
	u := -1
	for v >= 1000 && u < len(units)-1 {
		v /= 1000
		u++
	}
	prec := 1
	if v >= 10 {
		prec = 0
	}
	return strconv.FormatFloat(v, 'f', prec, 64) + " " + units[u]
}

package filepicker

import (
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func TestMain(m *testing.M) { gifttest.Main(m) }

// outcome records what a picker reported.
type outcome struct {
	picked    [][]string
	cancelled int
}

func (o *outcome) pick(paths []string) { o.picked = append(o.picked, paths) }
func (o *outcome) cancel()             { o.cancelled++ }

func (o *outcome) last(t *testing.T) []string {
	t.Helper()
	if len(o.picked) == 0 {
		t.Fatal("the picker reported nothing")
	}
	return o.picked[len(o.picked)-1]
}

// mountPicker puts v in a 420x720 window and waits for its first listing.
func mountPicker(t *testing.T, v gift.View) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{
		View:  ui.Window(v),
		Size:  geom.Sz(420, 720),
		Theme: ui.LightTheme(),
		Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	waitListed(t, h)
	return h
}

// waitListed runs frames until no directory is being read. The listing runs
// on a goroutine of its own, so this waits on the wall clock, briefly.
func waitListed(t *testing.T, h *gifttest.Harness) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.Settle()
		if !h.Exists(gifttest.ByKey("loading")) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a directory listing did not arrive within 5 s\n%s", h.Dump())
		}
		time.Sleep(time.Millisecond)
	}
}

// tapRow taps the row with the given title and waits for whatever it opened.
func tapRow(t *testing.T, h *gifttest.Harness, title string) {
	t.Helper()
	h.Find(gifttest.ByText(title).And(gifttest.ByType("ui.VirtualListRow"))).Tap()
	waitListed(t, h)
}

func tapKey(t *testing.T, h *gifttest.Harness, key string) {
	t.Helper()
	h.Find(gifttest.ByKey(key)).Tap()
	waitListed(t, h)
}

func TestOpenGoesIntoADirectoryUpAgainAndAlongThePathBar(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	h.Find(gifttest.ByText("b.txt")).AssertVisible()

	tapRow(t, h, "alpha")
	h.Find(gifttest.ByText("two.txt")).AssertVisible()
	h.AssertNone(gifttest.ByText("b.txt"))

	tapRow(t, h, "sub")
	h.Find(gifttest.ByText("x.jpg")).AssertVisible()

	tapKey(t, h, "crumb1") // "alpha"
	h.Find(gifttest.ByText("two.txt")).AssertVisible()

	tapKey(t, h, "up")
	h.Find(gifttest.ByText("b.txt")).AssertVisible()
	h.Find(gifttest.ByKey("up")).AssertDisabled()
}

func TestOpenSelectsOnTheFirstTapAndPicksOnTheSecond(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	h.Find(gifttest.ByKey("primary")).AssertDisabled()

	tapRow(t, h, "b.txt")
	if len(o.picked) != 0 {
		t.Fatalf("the first tap picked %v", o.picked)
	}
	h.Find(gifttest.ByKey("primary")).AssertEnabled()

	tapRow(t, h, "c.png")
	tapRow(t, h, "c.png")
	if got := o.last(t); !slices.Equal(got, []string{"c.png"}) {
		t.Fatalf("a second tap on c.png picked %v", got)
	}
}

func TestOpenPicksTheSelectionWithThePrimaryButtonAndPathsAreRelative(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Start("alpha"))
	tapRow(t, h, "two.txt")
	tapKey(t, h, "primary")
	if got := o.last(t); !slices.Equal(got, []string{"alpha/two.txt"}) {
		t.Fatalf("picked %v, want alpha/two.txt", got)
	}
}

func TestCancelAndEscapeBothCancel(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	tapKey(t, h, "cancel")
	h.Key(gift.KeyEscape)
	h.Settle()
	if o.cancelled != 2 {
		t.Fatalf("cancel and escape cancelled %d times, want 2", o.cancelled)
	}
	if len(o.picked) != 0 {
		t.Fatalf("cancelling picked %v", o.picked)
	}
}

func TestOpenMultiTogglesAndPicksInListingOrder(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Mode(OpenMulti))
	tapRow(t, h, "c.png")
	tapRow(t, h, "A.JPG")
	tapRow(t, h, "b.txt")
	tapRow(t, h, "b.txt") // and off again
	h.Find(gifttest.ByKey("primary")).AssertText("Open (2)")
	tapKey(t, h, "primary")
	if got := o.last(t); !slices.Equal(got, []string{"A.JPG", "c.png"}) {
		t.Fatalf("picked %v, want A.JPG and c.png in listing order", got)
	}
}

func TestExtensionsDisableTheOtherFiles(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Extensions("jpg"))
	tapRow(t, h, "b.txt")
	tapRow(t, h, "b.txt")
	if len(o.picked) != 0 {
		t.Fatalf("a .txt file was picked with Extensions(jpg): %v", o.picked)
	}
	h.Find(gifttest.ByKey("primary")).AssertDisabled()
}

func TestDirectoryPicksTheDirectoryItShows(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Mode(Directory))
	tapRow(t, h, "b.txt") // disabled: nothing
	if len(o.picked) != 0 {
		t.Fatalf("a file was picked in Directory mode: %v", o.picked)
	}
	tapRow(t, h, "alpha")
	h.Find(gifttest.ByKey("primary")).AssertText("Choose")
	tapKey(t, h, "primary")
	if got := o.last(t); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("picked %v, want alpha", got)
	}
}

func TestSaveChecksTheNameAndAsksBeforeReplacing(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Mode(Save).SuggestedName("new.txt"))
	h.Find(gifttest.ByKey("primary")).AssertEnabled()

	// A file that does not exist is saved straight away.
	tapKey(t, h, "primary")
	if got := o.last(t); !slices.Equal(got, []string{"new.txt"}) {
		t.Fatalf("picked %v, want new.txt", got)
	}

	// A tap on a file takes its name, and saving over it asks first.
	tapRow(t, h, "b.txt")
	tapKey(t, h, "primary")
	if len(o.picked) != 1 {
		t.Fatalf("saving over b.txt did not ask first: %v", o.picked)
	}
	h.Find(gifttest.ByText("Replace").And(gifttest.ByType("ui.Button"))).Tap()
	h.Settle()
	if got := o.last(t); !slices.Equal(got, []string{"b.txt"}) {
		t.Fatalf("replacing picked %v, want b.txt", got)
	}

	// The name of a folder, and a name with a slash, cannot be saved.
	for _, bad := range []string{"alpha", "a/b"} {
		h.Find(gifttest.ByKey("name")).Focus()
		h.Key(gift.KeyA, gift.ModControl)
		h.Key(gift.KeyA, gift.ModMeta)
		h.Key(gift.KeyBackspace)
		h.TypeText(bad)
		h.Settle()
		h.Find(gifttest.ByKey("primary")).AssertDisabled()
	}
}

func TestEnterInTheNameFieldSaves(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Mode(Save).Start("alpha"))
	h.Find(gifttest.ByKey("name")).Focus()
	h.TypeText("notes.md")
	h.Key(gift.KeyEnter)
	h.Settle()
	if got := o.last(t); !slices.Equal(got, []string{"alpha/notes.md"}) {
		t.Fatalf("enter saved %v, want alpha/notes.md", got)
	}
}

// lockedFS refuses to read one directory, the way a directory without the
// read permission does.
type lockedFS struct {
	fs.FS
	locked string
}

func (l lockedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == l.locked {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	return fs.ReadDir(l.FS, name)
}

func TestADirectoryThatCannotBeReadLeavesThePickerWhereItWas(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(lockedFS{testFS(), "Zeta"}, o.pick, o.cancel))
	tapRow(t, h, "Zeta")
	h.Find(gifttest.ByKey("error")).AssertText(`No permission to open "Zeta".`)
	h.Find(gifttest.ByText("b.txt")).AssertVisible()

	tapRow(t, h, "alpha")
	h.AssertNone(gifttest.ByKey("error"))
}

func TestAStartDirectoryThatCannotBeReadCannotBeChosen(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(lockedFS{testFS(), "Zeta"}, o.pick, o.cancel).Mode(Directory).Start("Zeta"))
	h.Find(gifttest.ByKey("error")).AssertVisible()
	h.Find(gifttest.ByKey("primary")).AssertDisabled()
	tapKey(t, h, "up")
	h.Find(gifttest.ByKey("primary")).AssertEnabled()
	h.AssertNone(gifttest.ByKey("error"))
}

// gatedFS holds the read of one directory until the test lets it go.
type gatedFS struct {
	fs.FS
	slow string
	gate chan struct{}
}

func (g gatedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == g.slow {
		<-g.gate
	}
	return fs.ReadDir(g.FS, name)
}

// TestAListingThatArrivesLateIsDroppedWhenThePersonHasMovedOn: the person
// opens a slow directory, gives up and goes back along the path bar; when the
// slow one arrives it must not replace what is on the screen.
func TestAListingThatArrivesLateIsDroppedWhenThePersonHasMovedOn(t *testing.T) {
	var o outcome
	g := gatedFS{FS: testFS(), slow: "alpha/sub", gate: make(chan struct{})}
	h := mountPicker(t, New(g, o.pick, o.cancel).Start("alpha"))

	h.Find(gifttest.ByText("sub")).Tap()
	h.Settle()
	h.AssertExists(gifttest.ByKey("loading"))
	tapKey(t, h, "crumb0")
	h.Find(gifttest.ByText("b.txt")).AssertVisible()

	close(g.gate)
	time.Sleep(20 * time.Millisecond)
	h.Settle()
	h.Settle()
	h.Find(gifttest.ByText("b.txt")).AssertVisible()
	h.AssertNone(gifttest.ByText("x.jpg"))
}

// TestRowsCannotBeTappedWhileADirectoryIsRead is the review finding behind
// the rule in tap: a file selected in the listing that is about to go away
// either vanished from the pick, so that onPick got nothing, or named the
// file of the same name in the directory that arrived.
func TestRowsCannotBeTappedWhileADirectoryIsRead(t *testing.T) {
	fsys := testFS()
	fsys["Zeta/b.txt"] = &fstest.MapFile{Data: []byte("same name")}
	g := gatedFS{FS: fsys, slow: "Zeta", gate: make(chan struct{})}
	var o outcome
	h := mountPicker(t, New(g, o.pick, o.cancel).Mode(OpenMulti))

	h.Find(gifttest.ByText("Zeta")).Tap()
	h.Settle()
	h.Find(gifttest.ByText("b.txt")).Tap()
	h.Settle()
	close(g.gate)
	waitListed(t, h)

	h.Find(gifttest.ByText("one.txt")).AssertVisible()
	h.Find(gifttest.ByKey("primary")).AssertDisabled()
	h.Find(gifttest.ByKey("primary")).Tap()
	h.Settle()
	if len(o.picked) != 0 {
		t.Fatalf("a tap made while Zeta was read led to the pick %v", o.picked)
	}
}

func TestSaveCannotSaveIntoAStartDirectoryThatCannotBeRead(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(lockedFS{testFS(), "Zeta"}, o.pick, o.cancel).
		Mode(Save).Start("Zeta").SuggestedName("new.txt"))
	h.Find(gifttest.ByKey("error")).AssertVisible()
	h.Find(gifttest.ByKey("primary")).AssertDisabled()
	h.Find(gifttest.ByKey("name")).Focus()
	h.Key(gift.KeyEnter)
	h.Settle()
	if len(o.picked) != 0 {
		t.Fatalf("saved into an unreadable directory: %v", o.picked)
	}
}

func TestEscapeCancelsWithNothingFocused(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	h.AssertNoFocus()
	h.Key(gift.KeyEscape)
	h.Settle()
	if o.cancelled != 1 {
		t.Fatalf("escape with nothing focused cancelled %d times, want 1", o.cancelled)
	}
}

func TestAModeOutOfRangeIsRefusedByName(t *testing.T) {
	defer func() {
		r := recover()
		if msg, _ := r.(string); msg == "" || !strings.Contains(msg, "Mode(9)") {
			t.Fatalf("Mode(9) panicked with %v, want a message that names it", r)
		}
	}()
	New(testFS(), func([]string) {}, nil).Mode(Mode(9))
}

func TestANameIsCheckedAgainstTheIndexOfTheListing(t *testing.T) {
	l, _ := listDir(testFS(), ".", rules{})
	if isDir, ok := l.index["alpha"]; !ok || !isDir {
		t.Error("alpha is not indexed as a directory")
	}
	if isDir, ok := l.index["b.txt"]; !ok || isDir {
		t.Error("b.txt is not indexed as a file")
	}
	if _, ok := l.index[".hidden"]; ok {
		t.Error("a hidden entry that is not listed is indexed")
	}
}

func TestTheFilterNarrowsTheListAndSaysWhenNothingMatches(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	h.Find(gifttest.ByKey("filter")).Focus()
	h.TypeText("ALP")
	h.Settle()
	h.Find(gifttest.ByText("alpha")).AssertVisible()
	h.AssertNone(gifttest.ByText("b.txt"))
	h.TypeText("zzz")
	h.Settle()
	h.Find(gifttest.ByKey("empty")).AssertVisible()
}

// TestAHundredThousandFilesAreListedWithoutOverflowOrAFullPool is the reason
// the picker is built on ui.VirtualList.
func TestAHundredThousandFilesAreListedWithoutOverflowOrAFullPool(t *testing.T) {
	big := fstest.MapFS{}
	for i := range 100000 {
		big["f"+strconv.Itoa(i)+".jpg"] = &fstest.MapFile{}
	}
	var o outcome
	h := mountPicker(t, New(big, o.pick, o.cancel))
	h.AssertNoOverflow()
	n := len(h.FindAll(gifttest.ByType("ui.VirtualListRow")))
	if n == 0 || n > 40 {
		t.Fatalf("a hundred thousand files mounted %d rows", n)
	}
	h.Find(gifttest.ByKey("list")).ScrollBy(500000)
	h.Settle()
	h.AssertNoOverflow()
}

// TestThePickerLooksTheWayItLooks is the picture of the picker in both
// themes: a directory with folders, files of several kinds, one selected file
// and one that the extensions rule out. It only compares under the giftgpu
// build tag.
func TestThePickerLooksTheWayItLooks(t *testing.T) {
	for _, theme := range []struct {
		name string
		t    ui.Theme
	}{{"light", ui.LightTheme()}, {"dark", ui.DarkTheme()}} {
		var o outcome
		h := gifttest.New(t, gifttest.Options{
			View: ui.Window(New(testFS(), o.pick, o.cancel).
				Mode(OpenMulti).
				Extensions(".jpg", ".png")),
			Size:  geom.Sz(420, 720),
			Theme: theme.t,
			Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
		})
		waitListed(t, h)
		tapRow(t, h, "A.JPG")
		h.AssertNoOverflow()
		h.AssertGolden("picker-" + theme.name)
	}
}

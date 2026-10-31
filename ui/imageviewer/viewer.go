package imageviewer

import (
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui/motion"
)

// Origin is where the pictures of a viewer are shown small: a gallery, a row
// of thumbnails, a filmstrip. A viewer opens out of it and closes back into
// it, and keeps it pointed at the picture it shows. [ui.Gallery] is one.
type Origin interface {
	// ThumbnailFrame is where the thumbnail of id was drawn in the last
	// frame, in device space, with its corner radius, and whether it was
	// drawn at all.
	ThumbnailFrame(id asset.ID) (geom.Rect, float32, bool)
	// Conceal hides the thumbnail of id while the viewer shows the picture
	// somewhere else; the empty ID shows them all again.
	Conceal(id asset.ID)
	// Reveal brings the thumbnail of id into view, so that the viewer can
	// close into it.
	Reveal(id asset.ID) bool
}

// The springs of the hero transition. Opening has a trace of overshoot,
// which is what makes a picture that grows out of its tile feel thrown
// rather than slid; closing has none, because a picture that shrinks past
// its tile and grows back reads as a mistake.
var (
	openSpring  = motion.Spring{Response: 420 * time.Millisecond, Damping: 0.86}
	closeSpring = motion.Spring{Response: 380 * time.Millisecond, Damping: 1}
)

type phase uint8

const (
	closed phase = iota
	opening
	open
	closing
)

// dragMode is what a one finger drag is moving.
type dragMode uint8

const (
	dragNone dragMode = iota
	// dragPage moves the pages.
	dragPage
	// dragPan pans a zoomed picture and hands what goes past its edge on
	// to the pages.
	dragPan
	// dragSwipe drags the picture down to close.
	dragSwipe
	// dragCard drags the card up or down.
	dragCard
)

// Viewer is the retained model of a full screen picture viewer: the
// catalogue it pages through, the picture it shows, and everything the
// gestures leave behind between two frames — the zoom, the position of the
// pages, a picture half dragged away.
//
// It belongs to the application, like a [ui.Gallery] and for the same
// reason: it has to outlive every build, and the application has to be able
// to open and close it from anywhere. [Fullscreen] is the view of it.
//
//	viewer := imageviewer.New(gallery.Collection(), gallery.Sources())
//	viewer.SetOrigin(gallery)
//	...
//	ui.ZStack(
//		ui.ImageGallery(gallery).OnActivate(func(id asset.ID) { viewer.Open(id) }),
//		imageviewer.Fullscreen(viewer),
//	)
//
// A Viewer belongs to the UI executor and is not safe for concurrent use.
type Viewer struct {
	coll     *asset.Collection
	sources  func(asset.ID) asset.Source
	origin   Origin
	onChange func(asset.ID)
	onClose  func()

	phase phase
	index int
	id    asset.ID

	// askOpen, askClose and askTurn are what the application asked for
	// between two frames; the view does it at its next layout, where it has
	// the clock.
	askOpen, askClose bool
	askTurn           int

	// hero is the transition between the thumbnail and the screen.
	heroT0    time.Duration
	heroFrom  geom.Rect // the thumbnail, in device space
	heroR     float32
	heroHas   bool
	heroStart geom.Rect // closing: the frame the picture left from, local
	heroImage geom.Rect // closing a zoomed picture: its whole rectangle
	heroClip  bool      // closing a zoomed picture: crop by clipping
	heroAlpha float64   // closing: the background's alpha when it started

	pg       pager
	zoom     zoomer
	out      zoomer // the page that is leaving, while it is still visible
	outDir   int    // where the leaving page is relative to index, or 0
	st       [3]stages
	vw, vh   float64
	density  float32
	spacing  float64
	hasCard  bool
	closable bool

	mode     dragMode
	dragging bool
	pinching bool
	trans    geom.Point
	mag      float64
	anchor   geom.Point
	sw       swipe
	cd       card

	chrome    bool
	chromeT0  time.Duration
	hoverFrom time.Duration
	hoverAt   time.Duration
	hovering  bool
	wheelX    float32

	// bump rebuilds the view, inval lays it out again and cardInval lays out
	// the card; all three are handed over by the view.
	bump, inval, cardInval func()
}

// New returns a closed viewer over the catalogue c, whose entries sources
// resolves to something the image pipeline can open — the same resolver a
// [ui.Gallery] is given.
func New(c *asset.Collection, sources func(asset.ID) asset.Source) *Viewer {
	return &Viewer{coll: c, sources: sources, zoom: newZoomer(), out: newZoomer(), closable: true}
}

// SetOrigin sets where the viewer opens from and closes into. Nil opens and
// closes with a fade.
func (v *Viewer) SetOrigin(o Origin) { v.origin = o }

// OnChange sets the function called when the viewer shows a different
// picture, after a page was turned, and when it opens.
func (v *Viewer) OnChange(fn func(asset.ID)) { v.onChange = fn }

// OnClose sets the function called when the viewer has closed, after the
// transition back.
func (v *Viewer) OnClose(fn func()) { v.onClose = fn }

// Collection returns the catalogue the viewer pages through.
func (v *Viewer) Collection() *asset.Collection { return v.coll }

// IsOpen reports whether the viewer is on the screen, opening or closing
// included.
func (v *Viewer) IsOpen() bool { return v.phase != closed }

// Current returns the entry the viewer shows, and whether it is open.
func (v *Viewer) Current() (asset.ID, bool) { return v.id, v.phase != closed }

// Open shows the entry id, growing it out of its thumbnail in the origin. It
// reports false for an ID the catalogue does not have.
func (v *Viewer) Open(id asset.ID) bool {
	i, ok := v.coll.Find(id)
	if !ok {
		return false
	}
	v.index, v.id = i, id
	v.phase, v.askOpen, v.askClose, v.askTurn = opening, true, false, 0
	v.heroHas = false
	if v.origin != nil {
		v.heroFrom, v.heroR, v.heroHas = v.origin.ThumbnailFrame(id)
		v.origin.Conceal(id)
	}
	v.zoom.reset()
	v.out.reset()
	v.outDir = 0
	v.pg = pager{}
	v.sw = swipe{}
	v.cd = card{}
	v.mode, v.dragging, v.pinching = dragNone, false, false
	v.chrome, v.chromeT0 = true, 0
	if v.onChange != nil {
		v.onChange(id)
	}
	v.changed()
	return true
}

// Close shrinks the picture back into its thumbnail and closes the viewer.
func (v *Viewer) Close() {
	if v.phase == closed || v.phase == closing {
		return
	}
	v.askClose = true
	v.changed()
}

// Next turns to the next picture, as the right arrow key does, and reports
// whether there is one.
func (v *Viewer) Next() bool { return v.ask(1) }

// Previous turns to the previous picture.
func (v *Viewer) Previous() bool { return v.ask(-1) }

func (v *Viewer) ask(dir int) bool {
	if v.phase == closed || !v.has(v.index+dir) {
		return false
	}
	v.askTurn = dir
	v.changed()
	return true
}

// has reports whether the catalogue has a position i.
func (v *Viewer) has(i int) bool { return i >= 0 && i < v.coll.Len() }

func (v *Viewer) changed() {
	if v.bump != nil {
		v.bump()
	}
	if v.inval != nil {
		v.inval()
	}
}

// source is the source of catalogue position i, or nil.
func (v *Viewer) source(i int) asset.Source {
	if !v.has(i) || v.sources == nil {
		return nil
	}
	return v.sources(v.coll.ID(i))
}

// sync follows the catalogue: the entry shown keeps being shown where it
// moved to, and a viewer whose entry was removed shows its neighbour.
func (v *Viewer) sync() {
	if v.has(v.index) && v.coll.ID(v.index) == v.id {
		return
	}
	if i, ok := v.coll.Find(v.id); ok {
		v.index = i
		return
	}
	v.index = max(0, min(v.index, v.coll.Len()-1))
	if v.has(v.index) {
		v.id = v.coll.ID(v.index)
	}
}

// pitch is the distance between two pages.
func (v *Viewer) pitch() float64 { return v.vw + v.spacing }

// turn shows the page dir away from the current one. The pages move from
// where they are, off, with velocity vel towards the next page, and the
// picture that leaves keeps its zoom while it is still visible.
func (v *Viewer) turn(now time.Duration, dir int, off, vel float64) {
	if dir == 0 || !v.has(v.index+dir) {
		v.pg.spring(now, off, vel)
		return
	}
	v.out = v.zoom
	v.outDir = -dir
	v.zoom = newZoomer()
	v.zoom.maxZoom = v.out.maxZoom
	v.st[1+dir], v.st[1], v.st[1-dir] = stages{}, v.st[1+dir], v.st[1]
	v.index += dir
	v.id = v.coll.ID(v.index)
	v.pg.spring(now, off-float64(dir)*v.pitch(), vel)
	v.cd = card{}
	if v.origin != nil {
		v.origin.Reveal(v.id)
		v.origin.Conceal(v.id)
	}
	if v.onChange != nil {
		v.onChange(v.id)
	}
	v.changed()
}

// jump shows position i at once.
func (v *Viewer) jump(i int) {
	if !v.has(i) || i == v.index {
		return
	}
	v.index = i
	v.id = v.coll.ID(i)
	v.zoom = newZoomer()
	v.outDir = 0
	v.pg = pager{}
	v.cd = card{}
	v.st = [3]stages{}
	if v.origin != nil {
		v.origin.Reveal(v.id)
		v.origin.Conceal(v.id)
	}
	if v.onChange != nil {
		v.onChange(v.id)
	}
	v.changed()
}

// heroProgress is the progress of the hero transition at now and whether it
// is still moving.
func (v *Viewer) heroProgress(now time.Duration) (float64, bool) {
	s := openSpring
	if v.phase == closing {
		s = closeSpring
	}
	t := now - v.heroT0
	f, _ := s.At(t, 0, 1, 0)
	return f, !s.Settled(t, 0, 1, 0, 0.002)
}

// settleHero moves an opening viewer to open once its transition is over.
func (v *Viewer) settleHero(now time.Duration) {
	if v.phase == opening && !v.askOpen {
		if _, moving := v.heroProgress(now); !moving {
			v.phase = open
		}
	}
}

// heroDuration bounds a hero transition.
func heroDuration() time.Duration { return openSpring.Duration(1, 0, 0.002) + 50*time.Millisecond }

// finishClose is the end of the transition back.
func (v *Viewer) finishClose() {
	v.phase = closed
	v.st = [3]stages{}
	if v.origin != nil {
		v.origin.Conceal("")
	}
	if v.onClose != nil {
		v.onClose()
	}
	v.changed()
}

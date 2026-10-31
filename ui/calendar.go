package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/gesture"
	"github.com/worldiety/gift/ui/motion"
)

var (
	calendarType      = gift.RegisterType("ui.Calendar")
	calendarCoreType  = gift.RegisterType("ui.calendarCore")
	calendarMonthType = gift.RegisterType("ui.calendarMonth")
	calendarArrowType = gift.RegisterType("ui.calendarArrow")
)

// Metrics of a calendar, in logical pixels: the ones of iOS's inline date
// picker.
const (
	calHeader   = float32(44)
	calWeekdays = float32(24)
	calRow      = float32(44)
	calRows     = 6
	calPad      = float32(12)
	calCircle   = float32(40)
	calDay      = float32(19)
	calWeekday  = float32(12)
	calTitle    = float32(17)
)

// calSpring settles the months after a swipe or a button.
var calSpring = motion.Spring{Response: 380 * time.Millisecond, Damping: 1}

// EnglishWeekdays are the default names of the weekday row, Sunday first as
// [time.Weekday] counts.
var EnglishWeekdays = [7]string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}

// GermanWeekdays are the German names, for [CalendarView.WeekdayNames].
var GermanWeekdays = [7]string{"SO", "MO", "DI", "MI", "DO", "FR", "SA"}

// CalendarView chooses a date on a month grid. It is created by [Calendar].
type CalendarView struct {
	value      Date
	bind       gift.Binding[Date]
	onChange   func(Date)
	min, max   Date
	first      time.Weekday
	months     [12]string
	weekdays   [7]string
	marked     func(Date) bool
	background Color
	hasBg      bool
	key, label string
	flex       float32
	width      float32
}

// Calendar returns a date picker of a month grid, the counterpart of
// SwiftUI's DatePicker with the graphical style and of UICalendarView.
//
//	day := ctx.State("day", ui.DateOf(2026, time.October, 31))
//	ui.Calendar().Bind(day.Binding()).FirstWeekday(time.Monday)
//
// The header names the month shown; a tap on it turns the grid into a month
// and a year wheel, and a second one turns it back. The arrows beside it, a
// swipe sideways, the wheel and page up and down change the month shown, which
// is not the date chosen: a tap on a day chooses it. Today is drawn in the
// accent colour and the chosen day in a filled circle of it. Days outside the
// [CalendarView.Range] are grey and refuse the tap. A focused calendar moves
// the chosen day with the arrow keys, by a day sideways and a week up and
// down.
func Calendar() CalendarView {
	return CalendarView{
		min: DateOf(1900, time.January, 1), max: DateOf(2100, time.December, 31),
		first: time.Monday, months: EnglishMonths, weekdays: EnglishWeekdays,
	}
}

// Value sets the chosen date.
func (v CalendarView) Value(d Date) CalendarView { v.value = d; return v }

// Bind reads the chosen date from b and writes the choice back.
func (v CalendarView) Bind(b gift.Binding[Date]) CalendarView { v.bind = b; return v }

// OnChange sets the function called with the date chosen.
func (v CalendarView) OnChange(fn func(Date)) CalendarView { v.onChange = fn; return v }

// Range limits the dates that can be chosen and the months that can be shown.
// The default is 1900 to 2100.
func (v CalendarView) Range(min, max Date) CalendarView {
	if !min.IsZero() && !max.IsZero() && !before(max, min) {
		v.min, v.max = min, max
	}
	return v
}

// FirstWeekday sets the day the weeks begin with. The default is Monday, as
// ISO 8601 and most of Europe have it.
func (v CalendarView) FirstWeekday(d time.Weekday) CalendarView { v.first = d; return v }

// MonthNames sets the names in the header, January first.
func (v CalendarView) MonthNames(names [12]string) CalendarView { v.months = names; return v }

// WeekdayNames sets the names of the weekday row, Sunday first; see
// [GermanWeekdays].
func (v CalendarView) WeekdayNames(names [7]string) CalendarView { v.weekdays = names; return v }

// Marked puts a dot under every day for which fn reports true: the days with
// appointments, the days a shop is open. It is asked for the days shown.
func (v CalendarView) Marked(fn func(Date) bool) CalendarView { v.marked = fn; return v }

// Background puts the calendar on a rounded card of colour c, which the
// month and year wheels fade into as well. Without it the calendar draws no
// background and the wheels fade into [ColorBackground], the colour of the
// window.
func (v CalendarView) Background(c Color) CalendarView { v.background, v.hasBg = c, true; return v }

// Width fixes the width. Without it the calendar fills what it is given, up
// to 480, and takes 354 along an unbounded axis.
func (v CalendarView) Width(px float32) CalendarView { v.width = max(0, px); return v }

// Key sets the reconciliation key.
func (v CalendarView) Key(s string) CalendarView { v.key = s; return v }

// Label sets the accessible name.
func (v CalendarView) Label(s string) CalendarView { v.label = s; return v }

// Flex sets the flex factor in a stack.
func (v CalendarView) Flex(f float32) CalendarView { v.flex = f; return v }

// ViewType implements gift.View.
func (CalendarView) ViewType() gift.TypeID { return calendarType }

// Build implements gift.View.
func (v CalendarView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.calendar", v.body)}, Flex: v.flex}
}

// monthRef is a month of a year, the one a calendar shows.
type monthRef struct {
	year  int
	month time.Month
}

func (m monthRef) add(k int) monthRef {
	t := time.Date(m.year, m.month+time.Month(k), 1, 0, 0, 0, 0, time.UTC)
	return monthRef{t.Year(), t.Month()}
}

func (m monthRef) less(o monthRef) bool {
	return m.year < o.year || m.year == o.year && m.month < o.month
}

func (v CalendarView) chosen() Date {
	if !v.bind.IsZero() {
		return v.bind.Get()
	}
	return v.value
}

func (v CalendarView) choose(d Date) {
	if before(d, v.min) || before(v.max, d) {
		return
	}
	if !v.bind.IsZero() {
		v.bind.Set(d)
	}
	if v.onChange != nil {
		v.onChange(d)
	}
}

func (v CalendarView) body(ctx *gift.Context) gift.View {
	sel := v.chosen()
	start := sel
	if start.IsZero() {
		start = DateFrom(time.Now())
	}
	shown := ctx.State("shown", monthRef{start.Year(), start.Month()})
	wheels := ctx.State("wheels", false)
	ctx.Read(shown)
	ctx.Read(wheels)
	lo, hi := monthRef{v.min.Year(), v.min.Month()}, monthRef{v.max.Year(), v.max.Month()}
	m := shown.Get()
	if m.less(lo) {
		m = lo
	}
	if hi.less(m) {
		m = hi
	}
	return calendarCore{v: v, shown: m, setShown: shown.Set, wheels: wheels.Get(), setWheels: wheels.Set, lo: lo, hi: hi, sel: sel}
}

// --- the calendar --------------------------------------------------------------

type calendarCore struct {
	v         CalendarView
	shown     monthRef
	setShown  func(monthRef)
	wheels    bool
	setWheels func(bool)
	lo, hi    monthRef
	sel       Date
}

func (calendarCore) ViewType() gift.TypeID { return calendarCoreType }

// The children of a calendar, in order.
const (
	calChildTitle = iota
	calChildPrev
	calChildNext
	calChildWeekdays // seven of them
	calChildGrids    = calChildWeekdays + 7
)

func (c calendarCore) Build(*gift.BuildContext) gift.Element {
	accent := ResolveColor(ColorAccent)
	label := ResolveColor(ColorLabel)
	second := ResolveColor(ColorSecondaryLabel)
	titleColour := label
	if c.wheels {
		titleColour = accent
	}
	kids := []gift.View{
		Text(c.v.months[c.shown.month-1] + " " + strconv.Itoa(c.shown.year)).FontSize(calTitle).Foreground(titleColour).MaxLines(1).Key("calendar-title"),
		calArrow{dir: -1, enabled: !c.wheels && c.lo.less(c.shown), core: c},
		calArrow{dir: 1, enabled: !c.wheels && c.shown.less(c.hi), core: c},
	}
	for i := range 7 {
		wd := (int(c.v.first) + i) % 7
		kids = append(kids, Text(c.v.weekdays[wd]).FontSize(calWeekday).Foreground(second).MaxLines(1))
	}
	if c.wheels {
		years := c.hi.year - c.lo.year + 1
		kids = append(kids, WheelPicker(
			Wheel(12, func(i int) string { return c.v.months[i] }).Selected(int(c.shown.month)-1).Cyclic(true).Width(160).Key("calendar-month").
				OnChange(func(i int) { c.setShown(monthRef{c.shown.year, time.Month(i + 1)}) }),
			Wheel(years, func(i int) string { return strconv.Itoa(c.lo.year + i) }).Selected(c.shown.year-c.lo.year).Width(90).Key("calendar-year").
				OnChange(func(i int) { c.setShown(monthRef{c.lo.year + i, c.shown.month}) }),
		).Background(c.wheelBackground()))
	} else {
		today := DateFrom(time.Now())
		for k := -1; k <= 1; k++ {
			kids = append(kids, calMonth{core: c, m: c.shown.add(k), today: today, accent: accent, label: label, second: second})
		}
	}
	n := &calNode{c: c}
	n.gs = []gesture.Gesture{
		gesture.Drag().Axis(gesture.Horizontal).OnChanged(n.onDrag).OnEnded(n.onDragEnd).OnCancelled(n.onCancel),
		gesture.Tap().OnEnded(n.onTap),
		gesture.Wheel().OnChanged(n.onWheel),
	}
	return gift.Element{
		Key:        "ui.calendarCore",
		Label:      c.v.label,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Focusable:  true,
		Children:   kids,
	}
}

func (c calendarCore) wheelBackground() Color {
	if c.v.hasBg {
		return c.v.background
	}
	return ColorBackground
}

// calState is what a calendar keeps across builds: the position of the months
// under a finger and on their way to rest.
type calState struct {
	inited   bool
	shown    monthRef // the month the position is relative to
	off, raw float64  // pixels the months are moved towards the next one
	trans    float32
	dragging bool
	moving   bool
	t0       time.Duration
	from, v0 float64

	wheelAcc  float32
	lastWheel time.Duration
}

func (s *calState) at(now time.Duration) (float64, bool) {
	if !s.moving {
		return s.off, false
	}
	t := now - s.t0
	x, _ := calSpring.At(t, s.from, 0, s.v0)
	return x, !calSpring.Settled(t, s.from, 0, s.v0, 0.25)
}

func (s *calState) settle(now time.Duration) {
	s.off, _ = s.at(now)
	s.raw, s.moving = s.off, false
}

func (s *calState) spring(now time.Duration, from, v float64) {
	s.off, s.raw, s.from, s.v0, s.t0, s.moving = from, from, from, v, now, true
}

type calNode struct {
	c   calendarCore
	gs  []gesture.Gesture
	st  *calState
	ctx *gift.EventContext
	now time.Duration

	// grid is the rectangle of the day grid and cell the size of a day, set
	// by the layout.
	grid   geom.Rect
	cell   geom.Size
	titleW float32
}

func (n *calNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := ctx.Retained[calState]()
	w := n.c.v.width
	switch {
	case w > 0:
	case c.HasBoundedWidth():
		w = min(c.Max.W, 480)
	default:
		w = 354
	}
	h := calHeader + calWeekdays + calRow*calRows
	size := c.Constrain(geom.Sz(w, h))
	cw := (size.W - 2*calPad) / 7
	n.cell = geom.Sz(cw, calRow)
	n.grid = geom.Rc(calPad, calHeader+calWeekdays, size.W-calPad, size.H)

	switch {
	case !st.inited || n.c.wheels:
		st.inited, st.shown = true, n.c.shown
	case st.shown != n.c.shown && !st.dragging:
		// The month changed by other means than a swipe — an arrow, the
		// wheels, the application: slide the new one in from its side.
		dir := 1
		if n.c.shown.less(st.shown) {
			dir = -1
		}
		st.shown = n.c.shown
		off, _ := st.at(ctx.Now())
		st.spring(ctx.Now(), off-float64(dir)*float64(n.grid.Width()), 0)
		ctx.Animate(time.Second)
	}

	t := ctx.Measure(calChildTitle, geom.Constraints{Max: geom.Sz(size.W-2*calPad-120, calHeader)})
	n.titleW = t.W
	ctx.Place(calChildTitle, geom.Pt(calPad+4, (calHeader-t.H)/2))
	for i, x := range [...]float32{size.W - calPad - 88, size.W - calPad - 44} {
		ctx.Measure(calChildPrev+i, geom.Tight(geom.Sz(44, calHeader)))
		ctx.Place(calChildPrev+i, geom.Pt(x, 0))
	}
	for i := range 7 {
		s := ctx.Measure(calChildWeekdays+i, geom.Constraints{Max: geom.Sz(cw, calWeekdays)})
		ctx.Place(calChildWeekdays+i, geom.Pt(calPad+float32(i)*cw+(cw-s.W)/2, calHeader+(calWeekdays-s.H)/2))
	}
	if n.c.wheels {
		// The wheels take the weekday row as well, which they hide.
		area := size.H - calHeader
		s := ctx.Measure(calChildGrids, geom.Constraints{Max: geom.Sz(size.W, area)})
		ctx.Place(calChildGrids, geom.Pt((size.W-s.W)/2, calHeader+(area-s.H)/2))
	} else {
		for k := range 3 {
			ctx.Measure(calChildGrids+k, geom.Tight(n.grid.Size()))
			ctx.Place(calChildGrids+k, n.grid.Min)
		}
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

func (n *calNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[calState]()
	b := ctx.Bounds()
	accent := ResolveColor(ColorAccent)
	if n.c.v.hasBg {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: ResolveColor(n.c.v.background), CornerRadius: 13})
	}
	// The wheels hide the arrows and the weekdays, as on iOS.
	ctx.PaintChild(calChildTitle)
	if !n.c.wheels {
		for i := calChildPrev; i < calChildGrids; i++ {
			ctx.PaintChild(i)
		}
	}
	// The chevron after the title: pointing right while the grid shows,
	// down while the wheels do.
	if title := n.titleRect(b); !title.IsEmpty() {
		c := geom.Pt(title.Max.X+12, (title.Min.Y+title.Max.Y)/2)
		dir := geom.Pt(1, 0)
		if n.c.wheels {
			dir = geom.Pt(0, 1)
		}
		paintChevron(ctx, c, dir, 5, accent)
	}
	off, moving := st.at(ctx.Now())
	grid := n.grid.Translate(b.Min)
	if n.c.wheels {
		grid.Min.Y = b.Min.Y + calHeader
	}
	ctx.PushClip(deviceMapping(ctx)(grid))
	if n.c.wheels {
		ctx.PaintChild(calChildGrids)
	} else {
		pitch := float64(grid.Width())
		for k := range 3 {
			x := float32(float64(k-1)*pitch - off)
			if x <= -grid.Width() || x >= grid.Width() {
				continue
			}
			ctx.PushTransform(geom.Translate(geom.Pt(x, 0)))
			ctx.PaintChild(calChildGrids + k)
			ctx.PopTransform()
		}
	}
	ctx.PopClip()
	if ia := ctx.Interaction(); ia.FocusVisible {
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b.Inset(geom.InsetsAll(1)), Color: accent, CornerRadius: 12, StrokeWidth: 2})
	}
	if moving {
		ctx.Animate(100 * time.Millisecond)
	}
}

// titleRect is where the title was laid out in the calendar's bounds b.
func (n *calNode) titleRect(b geom.Rect) geom.Rect {
	return geom.Rc(b.Min.X+calPad, b.Min.Y, b.Min.X+calPad+n.titleW, b.Min.Y+calHeader)
}

// paintChevron draws a chevron about c pointing along dir, its arms reaching
// s to either side, out of two turned bars.
func paintChevron(ctx *gift.PaintContext, c, dir geom.Point, s float32, col Color) {
	bar := func(x0, y0, x1, y1 float32) {
		dx, dy := x1-x0, y1-y0
		l := float32(math.Hypot(float64(dx), float64(dy)))
		m := geom.Pt((x0+x1)/2, (y0+y1)/2)
		ang := float32(math.Atan2(float64(dy), float64(dx)))
		ctx.PushTransform(geom.Translate(geom.Pt(-m.X, -m.Y)).Mul(geom.Rotate(ang)).Mul(geom.Translate(m)))
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(m.X-l/2-1, m.Y-1.2, m.X+l/2+1, m.Y+1.2), Color: col, CornerRadius: 1.2})
		ctx.PopTransform()
	}
	// The tip lies s/2 ahead of c, the ends s/2 behind it and s to either
	// side.
	tip := geom.Pt(c.X+dir.X*s/2, c.Y+dir.Y*s/2)
	side := geom.Pt(-dir.Y, dir.X)
	a := geom.Pt(tip.X-dir.X*s+side.X*s, tip.Y-dir.Y*s+side.Y*s)
	z := geom.Pt(tip.X-dir.X*s-side.X*s, tip.Y-dir.Y*s-side.Y*s)
	bar(a.X, a.Y, tip.X, tip.Y)
	bar(z.X, z.Y, tip.X, tip.Y)
}

func (n *calNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	n.st, n.ctx, n.now = ctx.Retained[calState](), ctx, ctx.Now()
	defer func() { n.ctx = nil }()
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
	case gift.EventKeyDown:
		return n.key(e)
	}
	return gesture.Handle(ctx, e, n.gs)
}

func (n *calNode) moved() {
	n.ctx.Repaint()
	n.ctx.Animate(time.Second)
}

func (n *calNode) onDrag(v gesture.DragValue) {
	if n.c.wheels {
		return
	}
	st := n.st
	if v.Phase == gesture.Began {
		st.settle(n.now)
		st.dragging, st.trans = true, v.Translation.X
		return
	}
	d := v.Translation.X - st.trans
	st.trans = v.Translation.X
	st.raw -= float64(d)
	w := float64(n.grid.Width())
	switch {
	case st.raw < 0 && !n.c.lo.less(n.c.shown), st.raw > 0 && !n.c.shown.less(n.c.hi):
		st.off = motion.Rubber(st.raw, w)
	default:
		st.off = math.Max(-w, math.Min(w, st.raw))
	}
	n.moved()
}

// onDragEnd turns to the month the swipe went towards: a flick whatever the
// distance, a slow drag past half way.
func (n *calNode) onDragEnd(v gesture.DragValue) {
	if n.c.wheels {
		return
	}
	st := n.st
	st.dragging = false
	vel := -float64(v.Velocity.X)
	w := float64(n.grid.Width())
	dir := 0
	switch {
	case vel > 400 && st.off > -w/2:
		dir = 1
	case vel < -400 && st.off < w/2:
		dir = -1
	case math.Abs(vel) <= 400:
		dir = int(math.Round(st.off / w))
	}
	n.turn(dir, vel)
}

func (n *calNode) onCancel() {
	n.st.dragging = false
	n.turn(0, 0)
}

// turn shows the month dir away and lets the months move there from where
// they are.
func (n *calNode) turn(dir int, vel float64) {
	st := n.st
	off, _ := st.at(n.now)
	if dir > 0 && !n.c.shown.less(n.c.hi) || dir < 0 && !n.c.lo.less(n.c.shown) {
		dir = 0
	}
	dir = max(-1, min(1, dir))
	w := float64(n.grid.Width())
	st.spring(n.now, off-float64(dir)*w, vel)
	if dir != 0 {
		st.shown = n.c.shown.add(dir)
		n.c.setShown(st.shown)
	}
	n.moved()
}

func (n *calNode) onTap(v gesture.TapValue) {
	p := v.Local(v.Location)
	if p.Y < calHeader {
		if p.X < calPad+n.titleW+28 {
			n.c.setWheels(!n.c.wheels)
			n.ctx.RequestLayout()
		}
		return
	}
	if n.c.wheels || !n.grid.Contains(p) {
		return
	}
	col := int((p.X - n.grid.Min.X) / n.cell.W)
	row := int((p.Y - n.grid.Min.Y) / n.cell.H)
	if d, ok := n.dayAt(n.c.shown, row*7+col); ok {
		n.c.v.choose(d)
	}
}

// dayAt is the date in cell i of the grid of month m, and whether that cell
// holds a day of it.
func (n *calNode) dayAt(m monthRef, i int) (Date, bool) {
	lead := leadingCells(m, n.c.v.first)
	day := i - lead + 1
	if day < 1 || day > monthLength(m.year, m.month) {
		return Date{}, false
	}
	return DateOf(m.year, m.month, day), true
}

// leadingCells is how many cells of the first week come before the first
// day of m.
func leadingCells(m monthRef, first time.Weekday) int {
	wd := time.Date(m.year, m.month, 1, 0, 0, 0, 0, time.UTC).Weekday()
	return (int(wd) - int(first) + 7) % 7
}

// onWheel turns a month for every notch of a wheel, and once in a fifth of a
// second at most for the stream of small steps of a trackpad.
func (n *calNode) onWheel(v gesture.WheelValue) bool {
	if n.c.wheels {
		return false
	}
	st := n.st
	d := v.Delta.Y
	if math.Abs(float64(v.Delta.X)) > math.Abs(float64(d)) {
		d = -v.Delta.X
	}
	st.wheelAcc += d
	if math.Abs(float64(st.wheelAcc)) < 1 || st.lastWheel != 0 && n.now-st.lastWheel < 200*time.Millisecond {
		return true
	}
	dir := 1
	if st.wheelAcc > 0 {
		dir = -1
	}
	st.wheelAcc, st.lastWheel = 0, n.now
	n.turn(dir, 0)
	return true
}

func (n *calNode) key(e gift.Event) bool {
	move := func(days int) bool {
		d := n.c.sel
		if d.IsZero() {
			d = DateOf(n.c.shown.year, n.c.shown.month, 1)
			days = 0
		}
		t := time.Date(d.Year(), d.Month(), d.Day()+days, 0, 0, 0, 0, time.UTC)
		nd := DateOf(t.Year(), t.Month(), t.Day())
		if before(nd, n.c.v.min) || before(n.c.v.max, nd) {
			return true
		}
		n.c.v.choose(nd)
		if m := (monthRef{nd.Year(), nd.Month()}); m != n.c.shown {
			dir := 1
			if m.less(n.c.shown) {
				dir = -1
			}
			n.turn(dir, 0)
		}
		return true
	}
	switch e.Key {
	case gift.KeyLeft:
		return move(-1)
	case gift.KeyRight:
		return move(1)
	case gift.KeyUp:
		return move(-7)
	case gift.KeyDown:
		return move(7)
	case gift.KeyPageUp:
		n.turn(-1, 0)
		return true
	case gift.KeyPageDown:
		n.turn(1, 0)
		return true
	}
	return false
}

// --- the arrows ---------------------------------------------------------------

type calArrow struct {
	dir     int
	enabled bool
	core    calendarCore
}

func (calArrow) ViewType() gift.TypeID { return calendarArrowType }

func (a calArrow) Build(*gift.BuildContext) gift.Element {
	key, label := "calendar-next", "Next month"
	if a.dir < 0 {
		key, label = "calendar-previous", "Previous month"
	}
	n := &calArrowNode{a: a, colour: ResolveColor(ColorAccent), off: ResolveColor(ColorSecondaryLabel)}
	return gift.Element{Key: key, Label: label, Layouter: n, Painter: n, Interactor: n, Disabled: !a.enabled}
}

type calArrowNode struct {
	a           calArrow
	colour, off Color
}

func (n *calArrowNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(44, calHeader))
}

func (n *calArrowNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	col := n.colour
	if !n.a.enabled {
		col = scaleColor(n.off, 0.5)
	} else if ctx.Interaction().Pressed {
		col = scaleColor(col, 0.5)
	}
	paintChevron(ctx, geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2), geom.Pt(float32(n.a.dir), 0), 7, col)
}

// HandleEvent changes the month on a tap; the calendar slides it in on its
// next layout.
func (n *calArrowNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			n.a.core.setShown(n.a.core.shown.add(n.a.dir))
		}
		return true
	}
	return false
}

// --- one month -----------------------------------------------------------------

type calMonth struct {
	core                  calendarCore
	m                     monthRef
	today                 Date
	accent, label, second Color
}

func (calMonth) ViewType() gift.TypeID { return calendarMonthType }

func (c calMonth) Build(*gift.BuildContext) gift.Element {
	days := monthLength(c.m.year, c.m.month)
	onAccent := ResolveColor(ColorOnAccent)
	kids := make([]gift.View, days)
	for d := 1; d <= days; d++ {
		date := DateOf(c.m.year, c.m.month, d)
		col := c.label
		switch {
		case before(date, c.core.v.min) || before(c.core.v.max, date):
			col = scaleColor(c.second, 0.6)
		case date == c.core.sel:
			col = onAccent
		case date == c.today:
			col = c.accent
		}
		kids[d-1] = Text(strconv.Itoa(d)).FontSize(calDay).Foreground(col).MaxLines(1).Key("day-" + strconv.Itoa(d))
	}
	n := &calMonthNode{c: c}
	return gift.Element{Key: "month-" + strconv.Itoa(c.m.year) + "-" + strconv.Itoa(int(c.m.month)), Layouter: n, Painter: n, Children: kids}
}

type calMonthNode struct {
	c    calMonth
	cell geom.Size
	lead int
}

func (n *calMonthNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	size := c.Constrain(c.Max)
	n.cell = geom.Sz(size.W/7, size.H/calRows)
	n.lead = leadingCells(n.c.m, n.c.core.v.first)
	for i := range ctx.ChildCount() {
		s := ctx.Measure(i, geom.Constraints{Max: n.cell})
		k := n.lead + i
		x := float32(k%7)*n.cell.W + (n.cell.W-s.W)/2
		y := float32(k/7)*n.cell.H + (n.cell.H-s.H)/2
		ctx.Place(i, geom.Pt(x, y))
	}
	return size
}

func (n *calMonthNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c := n.c
	sel := c.core.sel
	r := min(calCircle, n.cell.W-2, n.cell.H-2) / 2
	centre := func(day int) geom.Point {
		k := n.lead + day - 1
		return geom.Pt(b.Min.X+(float32(k%7)+0.5)*n.cell.W, b.Min.Y+(float32(k/7)+0.5)*n.cell.H)
	}
	if !sel.IsZero() && sel.Year() == c.m.year && sel.Month() == c.m.month {
		p := centre(sel.Day())
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(p.X-r, p.Y-r, p.X+r, p.Y+r), Color: c.accent, CornerRadius: r})
	}
	if mark := c.core.v.marked; mark != nil {
		days := monthLength(c.m.year, c.m.month)
		for d := 1; d <= days; d++ {
			if !mark(DateOf(c.m.year, c.m.month, d)) {
				continue
			}
			p := centre(d)
			col := c.accent
			if DateOf(c.m.year, c.m.month, d) == sel {
				col = ResolveColor(ColorOnAccent)
			}
			ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(p.X-2.5, p.Y+r-7.5, p.X+2.5, p.Y+r-2.5), Color: col, CornerRadius: 2.5})
		}
	}
	ctx.PaintChildren()
}

package ui

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	searchFieldType = gift.RegisterType("ui.SearchField")
	searchBoxType   = gift.RegisterType("ui.searchBox")
	searchClearType = gift.RegisterType("ui.searchClear")
)

// Metrics of a search field: the ones of UISearchBar's text field.
const (
	searchHeight = float32(36)
	searchRadius = float32(10)
	searchIcon   = float32(16)
	searchInset  = float32(8)
	searchGap    = float32(6)
	searchClear  = float32(30)
)

// SearchFieldView is a field for a search term. It is created by
// [SearchField].
type SearchFieldView struct {
	ed          *TextEditor
	placeholder string
	name        string
	onChange    func(string)
	onSubmit    func(string)
	bind        gift.Binding[string]
	disabled    bool
	key         string
	flex        float32
}

// SearchField returns the search field of iOS: a [TextField] in a rounded
// box with a magnifying glass in front and, while it holds text, a button
// behind that clears it.
//
//	q := ctx.State("q", "")
//	ui.SearchField(ui.Editor(ctx, "q-editor", "")).Bind(q.Binding()).OnSubmit(search)
//
// The clear button leaves the focus and the on-screen keyboard where they
// are, so that a new term can be typed at once, and escape clears the field
// too, as on macOS; escape in an empty field goes on to whatever handles it
// around it, a sheet that closes, say. The placeholder is "Search" unless
// [SearchFieldView.Placeholder] says otherwise.
func SearchField(ed *TextEditor) SearchFieldView {
	if ed == nil {
		panic("gift/ui: SearchField with a nil *TextEditor; create one with ui.NewTextEditor " +
			"or, inside a component, with ui.Editor(ctx, name, initial)")
	}
	return SearchFieldView{ed: ed, placeholder: "Search"}
}

// Placeholder sets the text shown while the field is empty.
func (v SearchFieldView) Placeholder(s string) SearchFieldView { v.placeholder = s; return v }

// Label sets the accessible name; the placeholder is used without one.
func (v SearchFieldView) Label(s string) SearchFieldView { v.name = s; return v }

// OnChange sets the function called with the text after every edit,
// clearing included.
func (v SearchFieldView) OnChange(f func(string)) SearchFieldView { v.onChange = f; return v }

// OnSubmit sets the function called with the text when enter is pressed.
func (v SearchFieldView) OnSubmit(f func(string)) SearchFieldView { v.onSubmit = f; return v }

// Bind reads the text from b and writes every edit back.
func (v SearchFieldView) Bind(b gift.Binding[string]) SearchFieldView { v.bind = b; return v }

// Disabled takes the field out of input.
func (v SearchFieldView) Disabled(b bool) SearchFieldView { v.disabled = b; return v }

// Key sets the reconciliation key.
func (v SearchFieldView) Key(s string) SearchFieldView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v SearchFieldView) Flex(f float32) SearchFieldView { v.flex = f; return v }

// ViewType implements gift.View.
func (v SearchFieldView) ViewType() gift.TypeID { return searchFieldType }

// Build implements gift.View.
func (v SearchFieldView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.search", v.body)}}
}

// body rebuilds when the field goes from empty to not and back, which is when
// the clear button comes and goes; a keystroke that changes neither costs no
// build.
func (v SearchFieldView) body(ctx *gift.Context) gift.View {
	has := ctx.State("has", false)
	ctx.Read(has)
	if !v.bind.IsZero() {
		if s := v.bind.Get(); s != v.ed.Text() {
			v.ed.SetText(s)
		}
	}
	changed := func(s string) {
		if (s != "") != has.Get() {
			has.Set(s != "")
		}
		if v.onChange != nil {
			v.onChange(s)
		}
	}
	clear := func() {
		if v.ed.Len() == 0 {
			return
		}
		v.ed.SetText("")
		if !v.bind.IsZero() {
			v.bind.Set("")
		}
		changed("")
	}
	full := v.ed.Len() > 0
	if full != has.Get() {
		has.Set(full)
	}
	right := searchInset
	if full {
		right = searchClear
	}
	field := TextField(v.ed).
		Placeholder(v.placeholder).
		Label(v.name).
		OnChange(changed).
		OnSubmit(v.onSubmit).
		Disabled(v.disabled).
		FontSize(17).
		Background(ColorControl).
		Border(Border{}).
		CornerRadius(searchRadius).
		PaddingInsets(geom.Insets{Left: searchInset + searchIcon + searchGap, Right: right, Top: 5, Bottom: 5}).
		MinHeight(searchHeight)
	if !v.bind.IsZero() {
		field = field.Bind(v.bind)
	}
	return searchBox{field: field, full: full && !v.disabled, clear: clear}
}

// searchBox lays the field over the whole box and the glass and the clear
// button over its padding.
type searchBox struct {
	field TextFieldView
	full  bool
	clear func()
}

func (searchBox) ViewType() gift.TypeID { return searchBoxType }

func (b searchBox) Build(*gift.BuildContext) gift.Element {
	kids := []gift.View{b.field.Key("search-field")}
	if b.full {
		kids = append(kids, searchClearButton{clear: b.clear})
	}
	n := &searchBoxNode{clear: b.clear, full: b.full, glass: ResolveColor(ColorSecondaryLabel)}
	return gift.Element{Key: "ui.searchBox", Layouter: n, Painter: n, Interactor: n, Children: kids}
}

type searchBoxNode struct {
	clear func()
	full  bool
	glass Color
	h     float32
}

func (n *searchBoxNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Pt(0, 0))
	if ctx.ChildCount() > 1 {
		ctx.Measure(1, geom.Tight(geom.Sz(searchClear, s.H)))
		ctx.Place(1, geom.Pt(s.W-searchClear, 0))
	}
	n.h = s.H
	return s
}

// Paint draws the field and over it the magnifying glass: a ring and a
// handle at forty-five degrees, in the size of the glass of SF Symbols.
func (n *searchBoxNode) Paint(ctx *gift.PaintContext) {
	ctx.PaintChildren()
	b := ctx.Bounds()
	c := geom.Pt(b.Min.X+searchInset+searchIcon*0.42, b.Min.Y+n.h/2-searchIcon*0.08)
	r := searchIcon * 0.33
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: geom.Rc(c.X-r, c.Y-r, c.X+r, c.Y+r), Color: n.glass, CornerRadius: r, StrokeWidth: 1.8})
	d := float32(math.Sqrt2 / 2)
	paintBar(ctx, geom.Pt(c.X+r*d, c.Y+r*d), geom.Pt(c.X+r*d+searchIcon*0.26, c.Y+r*d+searchIcon*0.26), 1, n.glass)
}

// paintBar draws a bar with round ends from a to z, of half thickness w.
func paintBar(ctx *gift.PaintContext, a, z geom.Point, w float32, col Color) {
	dx, dy := z.X-a.X, z.Y-a.Y
	l := float32(math.Hypot(float64(dx), float64(dy)))
	m := geom.Pt((a.X+z.X)/2, (a.Y+z.Y)/2)
	ang := float32(math.Atan2(float64(dy), float64(dx)))
	ctx.PushTransform(geom.Translate(geom.Pt(-m.X, -m.Y)).Mul(geom.Rotate(ang)).Mul(geom.Translate(m)))
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(m.X-l/2-w, m.Y-w, m.X+l/2+w, m.Y+w), Color: col, CornerRadius: w})
	ctx.PopTransform()
}

// HandleEvent clears the field on escape while it holds text; the key
// arrives here because the field does not handle it.
func (n *searchBoxNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind == gift.EventKeyDown && e.Key == gift.KeyEscape && n.full {
		n.clear()
		ctx.Repaint()
		return true
	}
	return false
}

// searchClearButton is the round cross that clears the field.
type searchClearButton struct{ clear func() }

func (searchClearButton) ViewType() gift.TypeID { return searchClearType }

func (b searchClearButton) Build(*gift.BuildContext) gift.Element {
	n := &searchClearNode{clear: b.clear, disc: ResolveColor(Fade(ColorSecondaryLabel, 0.6)), cross: ResolveColor(ColorControl)}
	return gift.Element{
		Key:      "search-clear",
		Label:    "Clear",
		Layouter: n, Painter: n, Interactor: n,
		// A tap here keeps the field focused and the keyboard up.
		PreservesFocus: true,
	}
}

type searchClearNode struct {
	clear       func()
	disc, cross Color
}

func (n *searchClearNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(c.Max)
}

// Paint draws xmark.circle.fill: a grey disc with a cross cut out of it in
// the colour of the field, darker while pressed.
func (n *searchClearNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2-2, (b.Min.Y+b.Max.Y)/2)
	r := searchIcon / 2
	disc := n.disc
	if ctx.Interaction().Pressed {
		disc = scaleColor(disc, 0.7)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(c.X-r, c.Y-r, c.X+r, c.Y+r), Color: disc, CornerRadius: r})
	k := r * 0.38
	paintBar(ctx, geom.Pt(c.X-k, c.Y-k), geom.Pt(c.X+k, c.Y+k), 0.9, n.cross)
	paintBar(ctx, geom.Pt(c.X-k, c.Y+k), geom.Pt(c.X+k, c.Y-k), 0.9, n.cross)
}

func (n *searchClearNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			n.clear()
		}
		return true
	}
	return false
}

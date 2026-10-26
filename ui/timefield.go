package ui

import (
	"fmt"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
)

var timeFieldType = gift.RegisterType("ui.TimeField")

// TimeFieldView is a field for a time of day on a 24 hour clock, entered as
// two parts, the hour and the minute. It is created by [TimeField]; the zero
// value is not useful.
//
//	start := ctx.State("start", ui.TimeOf(9, 0))
//	ui.TimeField().Bind(start.Binding()).Step(15 * time.Minute).Label("Start")
//
// # What the keyboard does
//
// The field has one focus stop, and inside it one part at a time has the
// keyboard; it is drawn in the accent.
//
//   - Digits fill the active part and move on by themselves: "0930" is 09:30,
//     "93" is 09:3 and waiting for the second digit of the minutes, and a "1"
//     in the hour waits for [SegmentTypingPause] to see whether it becomes 1x.
//   - ":" or "." moves on to the minutes, left and right move between the
//     parts, home and end go to the first and the last.
//   - Up and down step the active part, the minutes by [TimeFieldView.Step].
//     The minutes land on a multiple of the step first and carry into the
//     hour: with a step of five minutes, up on 17:57 is 18:00 and down on
//     18:00 is 17:55. The day wraps around, from 23:55 up to 00:00 and back.
//     With shift they step the hours from either part, and the hour part
//     steps by itself, wrapping without touching the minutes. An empty part
//     starts at the current time.
//   - Backspace and delete empty the active part; backspace in an empty part
//     goes back to the hour.
//   - Paste takes "9:30", "09:30", "930" or "0930"; see [ParseTimeOfDay].
//     Copy and cut give "09:30".
//   - Enter calls [TimeFieldView.OnSubmit], tab moves on to the next control.
//
// A click selects the part under the pointer, and while the field has the
// focus the wheel steps the part under the pointer like the arrows do. An
// unfocused field leaves the wheel to the form that scrolls it.
//
// # Its value
//
// The value is a [TimeOfDay], and the zero TimeOfDay — no time — is a field
// with both parts empty. A field with one part empty has no value either, and
// reports the zero TimeOfDay; what it shows survives until the user fills the
// other part.
//
// The value is the application's when the field has one, through
// [TimeFieldView.Value] or [TimeFieldView.Bind], and every change the user
// makes is reported at once, like the text of a [TextFieldView]. Without
// either the field keeps the time itself and only reports it.
//
// A value outside [TimeFieldView.Range] is accepted and reported, and the
// field draws its border in [ColorDanger]: a time entered in the wrong order
// is something to show, not something to correct behind the user's back.
type TimeFieldView struct {
	base
	entryLook

	value    TimeOfDay
	hasValue bool
	bind     gift.Binding[TimeOfDay]

	onChange func(TimeOfDay)
	onSubmit func(TimeOfDay)

	min, max TimeOfDay
	step     int32
	invalid  bool
	ph       [2]string
}

// TimeField returns an empty time field. Give it a value with
// [TimeFieldView.Value] or [TimeFieldView.Bind].
func TimeField() TimeFieldView {
	return TimeFieldView{step: 1, ph: [2]string{"--", "--"}}
}

// ViewType implements gift.View.
func (v TimeFieldView) ViewType() gift.TypeID { return timeFieldType }

// Build implements gift.View. Reading the binding here is what registers the
// component as a dependency of it, as in [TextFieldView.Build].
func (v TimeFieldView) Build(*gift.BuildContext) gift.Element {
	n := &entryNode{
		kind:  entryTime,
		parts: 2,
		width: [3]int8{2, 2},
		sep:   ":",
		ph:    [3]string{v.ph[0], v.ph[1]},
		step:  v.step,
	}
	value := v.value
	if !v.bind.IsZero() {
		value = v.bind.Get()
	}
	if v.hasValue || !v.bind.IsZero() {
		n.controlled = true
		n.valueSet = !value.IsZero()
		n.value = [3]int32{int32(value.hour), int32(value.minute)}
	}
	if !v.min.IsZero() {
		n.lo, n.hasLo = int64(v.min.Minutes()), true
	}
	if !v.max.IsZero() {
		n.hi, n.hasHi = int64(v.max.Minutes()), true
	}
	n.invalid = v.invalid
	if !v.bind.IsZero() || v.onChange != nil {
		bind, onChange := v.bind, v.onChange
		n.emit = func(vals [3]int32, ok bool) {
			t := timeOfParts(vals, ok)
			if !bind.IsZero() {
				bind.Set(t)
			}
			if onChange != nil {
				onChange(t)
			}
		}
	}
	if onSubmit := v.onSubmit; onSubmit != nil {
		n.submit = func(vals [3]int32, ok bool) { onSubmit(timeOfParts(vals, ok)) }
	}
	name := v.name
	if name == "" {
		name = "Time"
	}
	return v.entryLook.build(n, v.base, name)
}

func timeOfParts(vals [3]int32, ok bool) TimeOfDay {
	if !ok {
		return TimeOfDay{}
	}
	return TimeOf(int(vals[0]), int(vals[1]))
}

// --- modifiers ---------------------------------------------------------------

// Value sets the time the field shows. The zero TimeOfDay is an empty field.
// A field with a value shows the application's value; see the type
// documentation. [TimeFieldView.Bind] takes precedence over it.
func (v TimeFieldView) Value(t TimeOfDay) TimeFieldView { v.value, v.hasValue = t, true; return v }

// Bind connects the field to a [gift.State] holding the time, in both
// directions, like [TextFieldView.Bind]. Binding and [TimeFieldView.OnChange]
// compose: both run, the binding first.
func (v TimeFieldView) Bind(b gift.Binding[TimeOfDay]) TimeFieldView { v.bind = b; return v }

// OnChange registers a callback that runs whenever the value changes, with the
// new value. A part emptied by the user is a change to the zero TimeOfDay; a
// digit that does not change the value — the first one typed into an empty
// field — is no change.
func (v TimeFieldView) OnChange(f func(TimeOfDay)) TimeFieldView { v.onChange = f; return v }

// OnSubmit registers a callback that runs when the user presses enter in the
// field, with the value, after any change the enter itself made — a lone "1"
// still waiting for its second digit is the hour 01 by then.
func (v TimeFieldView) OnSubmit(f func(TimeOfDay)) TimeFieldView { v.onSubmit = f; return v }

// Range sets the earliest and the latest time the field accepts. A time
// outside it is still entered and reported, and the field draws itself as
// invalid. The zero TimeOfDay on either side leaves that side open. It panics
// on a minimum after the maximum.
func (v TimeFieldView) Range(min, max TimeOfDay) TimeFieldView {
	if !min.IsZero() && !max.IsZero() && min.Minutes() > max.Minutes() {
		panic(fmt.Sprintf("gift/ui: TimeField.Range(%v, %v) ends before it starts", min, max))
	}
	v.min, v.max = min, max
	return v
}

// Step sets how far the arrows and the wheel move the minutes: a step of 15
// minutes goes 09:00, 09:15, 09:30, and from 09:07 up to 09:15 first, and
// from 09:45 on to 10:00. The default is one minute. Typing is not limited to the step.
//
// It panics on a step that is not a whole number of minutes between one and
// sixty.
func (v TimeFieldView) Step(d time.Duration) TimeFieldView {
	if d < time.Minute || d > time.Hour || d%time.Minute != 0 {
		panic(fmt.Sprintf("gift/ui: TimeField.Step(%v) must be whole minutes from 1m to 1h", d))
	}
	v.step = int32(d / time.Minute)
	return v
}

// Invalid draws the field as invalid, whatever its value: for a rule the
// field cannot know, such as an end that has to come after a start.
func (v TimeFieldView) Invalid(b bool) TimeFieldView { v.invalid = b; return v }

// Placeholders sets what an empty hour and an empty minute show, replacing
// "--" — "hh" and "mm", or a localised "SS" for Stunde. A placeholder wider
// than two digits makes its part wider, and the digits are centred in it.
func (v TimeFieldView) Placeholders(hour, minute string) TimeFieldView {
	v.ph = [2]string{hour, minute}
	return v
}

// Label sets the accessible name of the field. It is never drawn. Without one
// the name is "Time".
func (v TimeFieldView) Label(s string) TimeFieldView { v.name = s; return v }

// Disabled takes the field out of input, like [TextFieldView.Disabled].
func (v TimeFieldView) Disabled(b bool) TimeFieldView { v.disabled = b; return v }

// Font sets the font the digits are drawn with.
func (v TimeFieldView) Font(f Font) TimeFieldView { v.font = f; return v }

// FontSize sets the em size in pixels, and with it the height of the field.
func (v TimeFieldView) FontSize(s float32) TimeFieldView { v.setFontSize("TimeField", s); return v }

// Foreground sets the colour of the digits.
func (v TimeFieldView) Foreground(c Color) TimeFieldView { v.fg, v.hasFG = c, true; return v }

// PlaceholderColor sets the colour of the placeholder of an empty part, and of
// the separator while the whole field is empty, replacing
// [ColorSecondaryLabel].
func (v TimeFieldView) PlaceholderColor(c Color) TimeFieldView {
	v.placeholderFG, v.hasPlaceholderFG = c, true
	return v
}

// ActivePartColor sets the highlight behind the part that has the keyboard
// and the colour of its digits, replacing [ColorAccent] and [ColorOnAccent].
// They are set together because each is chosen to be read on the other. A
// transparent background leaves the foreground alone to mark the part.
func (v TimeFieldView) ActivePartColor(background, foreground Color) TimeFieldView {
	v.activeBG, v.activeFG, v.hasActive = background, foreground, true
	return v
}

// FocusRing sets the border drawn while the field holds the keyboard focus. A
// zero width border removes it.
func (v TimeFieldView) FocusRing(b Border) TimeFieldView {
	v.focusRing, v.hasFocusRing = b, true
	return v
}

// Padding sets the same padding on all four edges, replacing the default of
// 5 by 8 that a [TextFieldView] has too.
func (v TimeFieldView) Padding(f float32) TimeFieldView {
	v.setPadding(f)
	v.hasPadding = true
	return v
}

// PaddingInsets sets the padding per edge.
func (v TimeFieldView) PaddingInsets(i geom.Insets) TimeFieldView {
	v.setPaddingInsets(i)
	v.hasPadding = true
	return v
}

// Frame fixes both axes. The parts stay at the left of a wider field and the
// line is centred in a taller one.
func (v TimeFieldView) Frame(w, h float32) TimeFieldView { v.setFrame(w, h); return v }

// MinWidth raises the minimum width of the field.
func (v TimeFieldView) MinWidth(f float32) TimeFieldView { v.setMinWidth(f); return v }

// MinHeight raises the minimum height of the field.
func (v TimeFieldView) MinHeight(f float32) TimeFieldView { v.setMinHeight(f); return v }

// MaxWidth lowers the maximum width of the field.
func (v TimeFieldView) MaxWidth(f float32) TimeFieldView { v.setMaxWidth(f); return v }

// MaxHeight lowers the maximum height of the field.
func (v TimeFieldView) MaxHeight(f float32) TimeFieldView { v.setMaxHeight(f); return v }

// Background fills the bounds behind the parts, replacing [ColorControl].
func (v TimeFieldView) Background(b Background) TimeFieldView { v.setBackgroundSpec(b); return v }

// Border strokes the inside of the bounds, replacing the themed hairline. An
// invalid field still draws [ColorDanger].
func (v TimeFieldView) Border(b Border) TimeFieldView { v.setBorder(b); return v }

// Shadow draws a blurred copy of the field's box behind it.
func (v TimeFieldView) Shadow(s Shadow) TimeFieldView { v.setShadow(s); return v }

// CornerRadius rounds the background and the border.
func (v TimeFieldView) CornerRadius(f float32) TimeFieldView { v.setCornerRadius(f); return v }

// Clip is accepted only as Clip(true), which is what the field already does;
// see [TextFieldView.Clip].
func (v TimeFieldView) Clip(b bool) TimeFieldView {
	if !b {
		panic("gift/ui: Clip(false) on a TimeField; the field clips its parts to its bounds")
	}
	return v
}

// Key sets the reconciliation key of this view among its siblings.
func (v TimeFieldView) Key(s string) TimeFieldView { v.setKey(s); return v }

// Flex makes the field take a share of the remaining main axis space of its
// parent stack.
func (v TimeFieldView) Flex(f float32) TimeFieldView { v.setFlex(f); return v }

// --- the look both entry fields share -----------------------------------------

// entryLook is the part of [TimeFieldView] and [DateFieldView] that is the
// same in both: the name, the font and the colours, and the build step that
// turns them into the resolved fields of an [entryNode].
type entryLook struct {
	name     string
	disabled bool

	font Font
	size float32

	fg    Color
	hasFG bool

	placeholderFG    Color
	hasPlaceholderFG bool

	activeBG, activeFG Color
	hasActive          bool

	focusRing    Border
	hasFocusRing bool
	hasPadding   bool
}

func (l *entryLook) setFontSize(who string, s float32) {
	if !isFinite(s) || s <= 0 {
		panic("gift/ui: " + who + ".FontSize must be a finite number greater than zero")
	}
	l.size = s
}

// build resolves the look into n and returns the element, in the way
// [TextFieldView.Build] does it for a text field.
func (l entryLook) build(n *entryNode, b base, name string) gift.Element {
	size := l.size
	if size <= 0 {
		size = DefaultFontSize
	}
	st := b.style.resolved()
	if !b.style.isSet(bitBackground) {
		st.background = ResolveColor(defaultFieldBackground)
	}
	if !b.style.isSet(bitBorder) {
		st.border = resolveBorder(defaultFieldBorder)
	}
	if !b.style.isSet(bitRadius) {
		st.radius = defaultFieldRadius
	}
	n.disabled = l.disabled
	n.fr, n.st = b.frame, st
	n.pad = defaultFieldPadding
	if l.hasPadding {
		n.pad = b.pad
	}
	n.req = text.Request{Font: resolveFont(l.font), Size: size, MaxWidth: geom.Unbounded()}
	n.fg = ResolveColor(ColorLabel)
	if l.hasFG {
		n.fg = ResolveColor(l.fg)
	}
	n.placeholderFG = ResolveColor(ColorSecondaryLabel)
	if l.hasPlaceholderFG {
		n.placeholderFG = ResolveColor(l.placeholderFG)
	}
	n.activeBG = ResolveColor(defaultSegmentBackground)
	n.activeFG = ResolveColor(defaultSegmentForeground)
	if l.hasActive {
		n.activeBG, n.activeFG = ResolveColor(l.activeBG), ResolveColor(l.activeFG)
	}
	n.disabledFG = ResolveColor(Fade(ColorLabel, 0.4))
	n.disabledBG = ResolveColor(ColorControlDisabled)
	n.focusRing = resolveBorder(defaultFieldFocusRing)
	if l.hasFocusRing {
		n.focusRing = resolveBorder(l.focusRing)
	}
	n.invalidBorder = resolveBorder(defaultInvalidBorder)
	return gift.Element{
		Key:        b.key,
		Flex:       b.flex,
		Layouter:   n,
		Painter:    n,
		Label:      name,
		Interactor: n,
		Focusable:  !l.disabled,
		Disabled:   l.disabled,
		Clip:       true,
	}
}

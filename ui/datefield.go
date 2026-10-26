package ui

import (
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var dateFieldType = gift.RegisterType("ui.DateField")

// DateFieldView is a field for a calendar day, entered as three parts in the
// order day, month, year: 26.10.2026. It is created by [DateField]; the zero
// value is not useful.
//
//	day := ctx.State("day", ui.DateFrom(time.Now()))
//	ui.DateField().Bind(day.Binding()).Label("Date")
//
// It is a [TimeFieldView] with three parts, and everything that type says
// about the keyboard, the wheel, the focus and whose value it is holds here
// unchanged. What is particular to a date:
//
//   - The day is kept within the month: the 31st becomes the 30th when the
//     month is stepped to November, and the 29th of February the 28th when
//     the year is stepped to one that is not a leap year. Before the year is
//     entered the 29th is allowed.
//   - The year takes four digits. A year left with one or two — "26" — is in
//     this century, 2026, as soon as the keyboard leaves it.
//   - ".", "/", "-" and "," all move on to the next part, so a date typed
//     with any of the usual separators lands in the right parts.
//   - Paste takes "26.10.2026", "26.10.26", "2026-10-26" and "26102026"; see
//     [ParseDate]. Copy and cut give "26.10.2026".
//   - Up and down wrap the day and the month and stop at the ends of the
//     year. An empty part starts at today.
//
// The order of the parts is fixed. It is the order of the German, and most of
// the European, spelling of a date; a field in another order is a second
// kind of field and not a modifier on this one, because the separator, the
// placeholders and the paste rules all change with it.
type DateFieldView struct {
	base
	entryLook

	value    Date
	hasValue bool
	bind     gift.Binding[Date]

	onChange func(Date)
	onSubmit func(Date)

	min, max Date
	invalid  bool
	ph       [3]string
}

// DateField returns an empty date field. Give it a value with
// [DateFieldView.Value] or [DateFieldView.Bind].
func DateField() DateFieldView {
	return DateFieldView{ph: [3]string{"--", "--", "----"}}
}

// ViewType implements gift.View.
func (v DateFieldView) ViewType() gift.TypeID { return dateFieldType }

// Build implements gift.View; see [TimeFieldView.Build].
func (v DateFieldView) Build(*gift.BuildContext) gift.Element {
	n := &entryNode{
		kind:  entryDate,
		parts: 3,
		width: [3]int8{2, 2, 4},
		sep:   ".",
		ph:    v.ph,
	}
	value := v.value
	if !v.bind.IsZero() {
		value = v.bind.Get()
	}
	if v.hasValue || !v.bind.IsZero() {
		n.controlled = true
		n.valueSet = !value.IsZero()
		n.value = dateParts(value)
	}
	if !v.min.IsZero() {
		n.lo, n.hasLo = n.order(dateParts(v.min)), true
	}
	if !v.max.IsZero() {
		n.hi, n.hasHi = n.order(dateParts(v.max)), true
	}
	n.invalid = v.invalid
	if !v.bind.IsZero() || v.onChange != nil {
		bind, onChange := v.bind, v.onChange
		n.emit = func(vals [3]int32, ok bool) {
			d := dateOfParts(vals, ok)
			if !bind.IsZero() {
				bind.Set(d)
			}
			if onChange != nil {
				onChange(d)
			}
		}
	}
	if onSubmit := v.onSubmit; onSubmit != nil {
		n.submit = func(vals [3]int32, ok bool) { onSubmit(dateOfParts(vals, ok)) }
	}
	name := v.name
	if name == "" {
		name = "Date"
	}
	return v.entryLook.build(n, v.base, name)
}

func dateParts(d Date) [3]int32 { return [3]int32{int32(d.day), int32(d.month), int32(d.year)} }

func dateOfParts(vals [3]int32, ok bool) Date {
	if !ok {
		return Date{}
	}
	return Date{year: int16(vals[2]), month: int8(vals[1]), day: int8(vals[0])}
}

// --- modifiers ---------------------------------------------------------------

// Value sets the day the field shows. The zero Date is an empty field.
// [DateFieldView.Bind] takes precedence over it.
func (v DateFieldView) Value(d Date) DateFieldView { v.value, v.hasValue = d, true; return v }

// Bind connects the field to a [gift.State] holding the day, in both
// directions; see [TimeFieldView.Bind].
func (v DateFieldView) Bind(b gift.Binding[Date]) DateFieldView { v.bind = b; return v }

// OnChange registers a callback that runs whenever the value changes, with the
// new value; see [TimeFieldView.OnChange].
func (v DateFieldView) OnChange(f func(Date)) DateFieldView { v.onChange = f; return v }

// OnSubmit registers a callback that runs when the user presses enter in the
// field, with the value.
func (v DateFieldView) OnSubmit(f func(Date)) DateFieldView { v.onSubmit = f; return v }

// Range sets the first and the last day the field accepts; see
// [TimeFieldView.Range]. It panics on a first day after the last.
func (v DateFieldView) Range(min, max Date) DateFieldView {
	if !min.IsZero() && !max.IsZero() && max.String() < min.String() {
		panic(fmt.Sprintf("gift/ui: DateField.Range(%v, %v) ends before it starts", min, max))
	}
	v.min, v.max = min, max
	return v
}

// Invalid draws the field as invalid, whatever its value.
func (v DateFieldView) Invalid(b bool) DateFieldView { v.invalid = b; return v }

// Placeholders sets what an empty day, month and year show, replacing "--",
// "--" and "----" — "TT", "MM" and "JJJJ" in German. A placeholder wider than
// its digits makes its part wider.
func (v DateFieldView) Placeholders(day, month, year string) DateFieldView {
	v.ph = [3]string{day, month, year}
	return v
}

// Label sets the accessible name of the field. It is never drawn. Without one
// the name is "Date".
func (v DateFieldView) Label(s string) DateFieldView { v.name = s; return v }

// Disabled takes the field out of input, like [TextFieldView.Disabled].
func (v DateFieldView) Disabled(b bool) DateFieldView { v.disabled = b; return v }

// Font sets the font the digits are drawn with.
func (v DateFieldView) Font(f Font) DateFieldView { v.font = f; return v }

// FontSize sets the em size in pixels, and with it the height of the field.
func (v DateFieldView) FontSize(s float32) DateFieldView { v.setFontSize("DateField", s); return v }

// Foreground sets the colour of the digits.
func (v DateFieldView) Foreground(c Color) DateFieldView { v.fg, v.hasFG = c, true; return v }

// FocusRing sets the border drawn while the field holds the keyboard focus. A
// zero width border removes it.
func (v DateFieldView) FocusRing(b Border) DateFieldView {
	v.focusRing, v.hasFocusRing = b, true
	return v
}

// Padding sets the same padding on all four edges, replacing the default of
// 5 by 8 that a [TextFieldView] has too.
func (v DateFieldView) Padding(f float32) DateFieldView {
	v.setPadding(f)
	v.hasPadding = true
	return v
}

// PaddingInsets sets the padding per edge.
func (v DateFieldView) PaddingInsets(i geom.Insets) DateFieldView {
	v.setPaddingInsets(i)
	v.hasPadding = true
	return v
}

// Frame fixes both axes. The parts stay at the left of a wider field and the
// line is centred in a taller one.
func (v DateFieldView) Frame(w, h float32) DateFieldView { v.setFrame(w, h); return v }

// MinWidth raises the minimum width of the field.
func (v DateFieldView) MinWidth(f float32) DateFieldView { v.setMinWidth(f); return v }

// MinHeight raises the minimum height of the field.
func (v DateFieldView) MinHeight(f float32) DateFieldView { v.setMinHeight(f); return v }

// MaxWidth lowers the maximum width of the field.
func (v DateFieldView) MaxWidth(f float32) DateFieldView { v.setMaxWidth(f); return v }

// MaxHeight lowers the maximum height of the field.
func (v DateFieldView) MaxHeight(f float32) DateFieldView { v.setMaxHeight(f); return v }

// Background fills the bounds behind the parts, replacing [ColorControl].
func (v DateFieldView) Background(b Background) DateFieldView { v.setBackgroundSpec(b); return v }

// Border strokes the inside of the bounds, replacing the themed hairline. An
// invalid field still draws [ColorDanger].
func (v DateFieldView) Border(b Border) DateFieldView { v.setBorder(b); return v }

// Shadow draws a blurred copy of the field's box behind it.
func (v DateFieldView) Shadow(s Shadow) DateFieldView { v.setShadow(s); return v }

// CornerRadius rounds the background and the border.
func (v DateFieldView) CornerRadius(f float32) DateFieldView { v.setCornerRadius(f); return v }

// Clip is accepted only as Clip(true), which is what the field already does;
// see [TextFieldView.Clip].
func (v DateFieldView) Clip(b bool) DateFieldView {
	if !b {
		panic("gift/ui: Clip(false) on a DateField; the field clips its parts to its bounds")
	}
	return v
}

// Key sets the reconciliation key of this view among its siblings.
func (v DateFieldView) Key(s string) DateFieldView { v.setKey(s); return v }

// Flex makes the field take a share of the remaining main axis space of its
// parent stack.
func (v DateFieldView) Flex(f float32) DateFieldView { v.setFlex(f); return v }

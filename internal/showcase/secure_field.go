package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input walks the focus to the PIN field, past the user name, the
	// password and its eye button, and types four digits, the last of which
	// is still shown.
	register(Demo{
		Name: "secure-field", Group: "basic", Title: "Secure Field", Summary: "A password or a PIN, shown as dots.", Width: 520, Height: 340,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"focus"},{"op":"focus"},{"op":"focus"},{"op":"focus"},{"op":"text","text":"2041"},{"op":"waitFrames","frames":2}]}`,
	}, secureFieldDemo)
}

func secureFieldDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	user := ui.Editor(ctx, "user", "ada")
	password := ui.Editor(ctx, "password", "correct horse")
	pin := ui.Editor(ctx, "pin", "")
	show := ctx.State("show", false)

	labeled := func(title string, field gift.View) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}

	// The password next to an eye button that shows it in the clear: a
	// borderless field in a box that carries the face.
	eye := outline.Eye
	if ctx.Read(show) {
		eye = outline.EyeSlash
	}
	passwordBox := ui.HStack(
		ui.SecureField(password).
			Placeholder("Password").
			Revealed(show.Get()).
			Background(ui.ColorClear).
			Border(ui.Border{}).
			FocusRing(ui.Border{}).
			Flex(1),
		ui.Button(ui.Icon(eye).Size(16).Foreground(ui.ColorSecondaryLabel), func() { show.Set(!show.Get()) }).
			Background(ui.ColorClear).
			Border(ui.Border{}).
			Padding(4).
			Label("Show password"),
	).Align(geom.Center).
		PaddingInsets(geom.Insets{Right: 2}).
		Background(ui.ColorControl).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		CornerRadius(6)

	return ui.VStack(
		labeled("User name", ui.TextField(user)),
		labeled("Password", passwordBox),
		// A PIN typed on a touch screen shows its last digit for a moment.
		labeled("PIN", ui.SecureField(pin).Placeholder("4 digits").ShowsLastCharacter(true).Frame(120, geom.Unbounded())),
	).Gap(14).Frame(320, geom.Unbounded())
	// snippet:end
}

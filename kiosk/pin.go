package kiosk

import (
	"fmt"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

// pinPrompt asks for the PIN over everything, as iOS asks for the passcode
// of Screen Time, and calls done with whether it was right.
func (k *Kiosk) pinPrompt(ctx *gift.Context, done func(ok bool)) gift.View {
	entered := ctx.State("entered", "")
	message := ctx.State("message", "")
	ctx.Read(entered)
	ctx.Read(message)
	digits := k.store.digits(k.cfg.PIN)
	if wait := k.store.waiting(time.Now()); wait > 0 && message.Get() == "" {
		message.Set(lockedFor(wait))
	}
	type_ := func(r rune) {
		if k.store.waiting(time.Now()) > 0 || len(entered.Get()) >= digits {
			return
		}
		s := entered.Get() + string(r)
		entered.Set(s)
		if len(s) < digits {
			return
		}
		// The hash takes some milliseconds; it runs beside the user
		// interface, which shows the last dot filled meanwhile.
		go func() {
			v := k.store.tryPIN(s, k.cfg.PIN, time.Now())
			k.post(func() {
				entered.Set("")
				switch {
				case v.ok:
					message.Set("")
					done(true)
				case v.wait > 0:
					message.Set(lockedFor(v.wait))
				default:
					message.Set(fmt.Sprintf("Wrong passcode. %d failed attempts.", v.failed))
				}
			})
		}()
	}
	return pinScreen("Enter Passcode", message.Get(), digits, len(entered.Get()), type_,
		func() { entered.Set(dropLast(entered.Get())) }, func() { done(false) })
}

// pinSetup asks for a new PIN twice and calls done with it, or with the
// empty string when cancelled.
func (k *Kiosk) pinSetup(ctx *gift.Context, done func(pin string)) gift.View {
	first := ctx.State("first", "")
	entered := ctx.State("entered", "")
	message := ctx.State("message", "")
	six := ctx.State("six", false)
	ctx.Read(first)
	ctx.Read(entered)
	ctx.Read(message)
	ctx.Read(six)
	digits := 4
	if six.Get() {
		digits = 6
	}
	title := "Enter a New Passcode"
	if first.Get() != "" {
		title = "Verify the New Passcode"
	}
	type_ := func(r rune) {
		s := entered.Get() + string(r)
		if len(s) > digits {
			return
		}
		entered.Set(s)
		if len(s) < digits {
			return
		}
		entered.Set("")
		switch {
		case first.Get() == "":
			if err := checkPIN(s); err != nil {
				message.Set(err.Error())
				return
			}
			first.Set(s)
			message.Set("")
		case s == first.Get():
			done(s)
		default:
			first.Set("")
			message.Set("The passcodes did not match. Try again.")
		}
	}
	options := ui.Button(ui.Text(fmt.Sprintf("Use a %d-Digit Passcode", 10-digits)).FontSize(15).Foreground(ui.ColorAccent),
		func() {
			six.Set(!six.Get())
			entered.Set("")
			first.Set("")
		}).Background(ui.ColorClear).Border(ui.Border{}).Key("kiosk.pin.options")
	return ui.ZStack(
		fill{pinScreen(title, message.Get(), digits, len(entered.Get()), type_,
			func() { entered.Set(dropLast(entered.Get())) }, func() { done("") })},
		fill{ui.ZStack(options).Align(geom.Bottom).PaddingInsets(geom.Insets{Bottom: 12})},
	)
}

func dropLast(s string) string {
	if s == "" {
		return s
	}
	return s[:len(s)-1]
}

func lockedFor(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	switch {
	case m <= 1:
		return "Too many failed attempts. Try again in 1 minute."
	case m < 120:
		return fmt.Sprintf("Too many failed attempts. Try again in %d minutes.", m)
	}
	return fmt.Sprintf("Too many failed attempts. Try again in %d hours.", (m+30)/60)
}

// pinScreen is the screen of a passcode: a title, a dot per digit, filled
// as far as they are typed, a message, and a keypad large enough for a
// finger on a 5 inch panel.
func pinScreen(title, message string, digits, filled int, type_ func(rune), del, cancel func()) gift.View {
	var dots []gift.View
	for i := range digits {
		d := ui.Box().Frame(14, 14).CornerRadius(7).Border(ui.Border{Width: 1.5, Color: ui.ColorLabel})
		if i < filled {
			d = d.Background(ui.ColorLabel)
		}
		dots = append(dots, d)
	}
	key := func(label string, fn func()) gift.View {
		return ui.Button(ui.Text(label).FontSize(28), fn).Frame(72, 72).CornerRadius(36).
			Background(ui.ColorControl).Border(ui.Border{}).Key("kiosk.pin." + label)
	}
	digit := func(d rune) gift.View { return key(string(d), func() { type_(d) }) }
	rows := []gift.View{}
	for _, r := range []string{"123", "456", "789"} {
		var ks []gift.View
		for _, d := range r {
			ks = append(ks, digit(d))
		}
		rows = append(rows, ui.HStack(ks...).Gap(24))
	}
	plain := func(label string, fn func()) gift.View {
		return ui.Button(ui.Text(label).FontSize(17).Foreground(ui.ColorAccent), fn).Frame(72, 72).
			Background(ui.ColorClear).Border(ui.Border{}).Key("kiosk.pin." + strings.ToLower(label))
	}
	rows = append(rows, ui.HStack(plain("Cancel", cancel), digit('0'), plain("Delete", del)).Gap(24))
	msg := ui.Text(message).FontSize(14).Foreground(ui.ColorDanger).Key("kiosk.pin.message")
	return ui.ZStack(
		ui.VStack(
			ui.Text(title).FontSize(20).Key("kiosk.pin.title"),
			ui.HStack(dots...).Gap(18),
			msg,
			ui.VStack(rows...).Gap(14),
		).Gap(22).Align(geom.Center),
	).Align(geom.Center).Background(ui.ColorBackground).Key("kiosk.pin")
}

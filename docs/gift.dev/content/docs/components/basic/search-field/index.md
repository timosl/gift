---
title: Search Field
---

`ui.SearchField` is the search field of iOS: a [Text Field](../text-field/) in a rounded box with a magnifying glass
in front and, while it holds text, a round button behind it that clears it.

{{< gallery-shot alt="A focused search field holding er, with a clear button, above the two cities that contain it" >}}

{{< gallery-code >}}

## Usage

```go
q := ctx.State("q", "")
ui.SearchField(ui.Editor(ctx, "q-editor", "")).Bind(q.Binding()).OnSubmit(search)
```

Like a text field it edits a `ui.TextEditor`, and `Bind`, `OnChange` and `OnSubmit` work the same way. The
placeholder is "Search" unless `Placeholder` sets another one.

## Clearing

The clear button empties the field and calls `OnChange` with the empty string. It leaves the keyboard focus and the
on-screen keyboard where they are, so that a new term can be typed at once.

Escape clears the field too, as on macOS. Escape in an empty field goes on to whatever handles it around the field,
a sheet that closes, for example.

## Look

The field is 36 points tall with a corner radius of 10 on `ui.ColorControl`, with 17 point text, as on iOS. It takes
the width it is offered.

## API

{{< api "SearchFieldView" >}}

## Related

- [Text Field](../text-field/), the field it is built on

---
title: Swipe Actions
---

`ui.Swipe` makes any view a row that a finger or a mouse drags sideways to uncover actions behind it: the swipe
actions of `UITableView` and of SwiftUI's `swipeActions`. Use it for the rows of a [List](../list/) or of any other
column.

{{< gallery-shot alt="A list of four mails in a card; the second row is dragged open to the left and shows an orange Flag and a red Delete action" >}}

{{< gallery-code >}}

## Usage

```go
ui.Swipe(ui.Row(m.From).Subtitle(m.Subject)).
	Trailing(
		ui.SwipeAction("Delete", remove).Icon(outline.TrashBin).Destructive(),
		ui.SwipeAction("Flag", flag).Icon(outline.Flag),
	).
	Leading(ui.SwipeAction("Unread", markUnread).Tint(ui.ColorAccent)).
	Key(m.ID)
```

`Trailing` sets the actions a drag to the left uncovers, `Leading` the ones a drag to the right uncovers. The first
action of each edge sits at that edge. An action is at least 74 points wide, has white text and an optional symbol
above it, and is grey unless it is `Destructive`, which makes it `ui.ColorDanger`, or has a `Tint`.

Give each row a stable `Key`, so that a row stays open on its own mail and not on whatever moved into its place.

## Interaction

- A drag past half of the actions, or a flick, leaves the row open on them; a shorter one springs back.
- A tap on an action runs it and closes the row.
- While a row is open, a tap anywhere else only closes it and reaches nothing underneath, as on iOS.
- Dragged far through, the first action of that edge grows over the whole row and runs when the finger lifts.
  `FullSwipe(false)` turns that off; the row then only opens.
- A stroke that goes more up or down than sideways belongs to the list around the row, which scrolls. A button in the
  row still gets its tap: a drag only becomes a swipe once it is longer than a tap.

## Look

The content slides over the actions on a background of its own, `ui.ColorBackground` unless `Background` says
otherwise. Set it to the colour the row sits on, for example `ui.ColorSurface` inside a [Card](../../layout/card/).
A row fills the width it is given and is as tall as its content.

## API

{{< api "SwipeView" >}}

{{< api "SwipeActionView" >}}

## Related

- [List](../list/) and [Row](../row/), which the rows are usually made of
- [Pull to Refresh](../pull-to-refresh/), the other gesture of an iOS list
- [Context Menu](../../dialogs/context-menu/), for the actions of a row on a long press

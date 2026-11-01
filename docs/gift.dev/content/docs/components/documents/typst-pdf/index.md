---
title: Typst PDF
---

The Typst viewer shows the pages of a document, and the document writes the same pages as a PDF file, with
[Typst](https://typst.app)'s own PDF writer. What is on the screen is what is in the file: both come from one setting
of the source, and the glyphs on the screen are those of the fonts that are embedded in the PDF. Use it for an
invoice, a report or a ticket that the user checks on the screen and then saves, prints or sends.

{{< gallery-shot alt="An invoice page with a heading, a date, a table of items, hours and amounts with a total, and a line about payment, below an Export PDF button and the path of the written file" >}}

{{< gallery-code >}}

## Usage

```go
doc := typstview.Use(ctx, "invoice", source)
typstview.Viewer(doc).Flex(1)

pdf, err := doc.PDF()
```

The viewer is the [Typst Editor](../typst-editor/) without the toolbar and without input. Its pages follow the
source as it changes, so the same document can be edited in one place and shown in another.

`PDF` waits for the source that is being set and returns the pages of the last source that was set without an
error. It may be called on any goroutine; a long document is better exported on one, with the result posted back
to the user interface.

## API

{{< api pkg="typstview" type="ViewerView" >}}

## Related

- [Typst Editor](../typst-editor/) edits a document on its pages.
- [File Picker](../../dialogs/file-picker/) chooses where to save the file.

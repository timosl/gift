---
title: Typst
weight: 7
---

This example edits a document that [Typst](https://typst.app) sets, an offer with two tables, one of them longer
than a page. On the left is its source in a [Code Editor](../../components/basic/code-editor/), colored as Typst
and with the lines Typst reports an error on marked red; on the right are its pages in a
[Typst Editor](../../components/documents/typst-editor/). Both edit one document: type in either, and the other
follows. The status line says how long the last setting took, and a button writes the document as a PDF.

{{< gallery-shot alt="The source of an offer in a code editor with line numbers and colored markup on the left, and its first page with a heading, a paragraph and a table of items and prices on the right, below a toolbar" >}}

## Run it

From a checkout of the [repository](https://github.com/worldiety/gift):

```bash
go run ./cmd/example-typst
```

Typst is a Rust library. gift loads it at run time without cgo, and the example carries it for macOS on Apple
silicon and for Linux on amd64 and arm64, by importing package
[`typst/embedded`](https://github.com/worldiety/gift/tree/main/typst/embedded). On the first start it is unpacked
into the user's cache directory, which takes a fraction of a second.

`-lib` loads another build of the library, one built from
[`typst/ffi`](https://github.com/worldiety/gift/tree/main/typst/ffi) with `cargo build --release` say, and `-fonts`
adds the TrueType fonts of a directory for the document to use.

## What to try

- Click into the text of the page and type, then delete it again with backspace.
- Double click a word and make it strong with the toolbar, or turn a line into a heading.
- Add a closing parenthesis to a line of the source: the line turns red, the status names the error, and the pages
  keep the last version that was set.
- Lengthen the long table in the source, with `range(1, 200)`, and watch the pages being set again.
- Save the PDF and open it next to the window.

{{< example-code >}}

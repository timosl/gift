// Package typst sets paged documents with Typst and hands the pages to gift
// to draw. It is a spike: it proves the path from source to pixels and to
// PDF, and leaves out much of what a product needs; see "Not yet" below.
//
// # No cgo
//
// Typst is a Rust library. It is compiled into libgifttypst, a shared library
// with a small C ABI, from the crate in typst/ffi, and opened at run time
// with purego, the way asset/turbojpeg opens libturbojpeg. gift itself keeps
// building with CGO_ENABLED=0; a system without the library reports
// [ErrUnavailable] and nothing else changes.
//
//	(cd typst/ffi && cargo build --release)
//
// The library is looked for at the path given to [Open], in GIFT_TYPST_LIB,
// next to the executable and on the dynamic linker's search path.
//
// # What crosses the boundary
//
// Into the library go font files and the source. An [Engine] keeps both
// between compilations, and Typst memoizes its intermediate results, so a
// compile after an edit redoes only what the edit touched.
//
// Out of it comes each page as a flat display list in points: glyph runs,
// filled rectangles — every horizontal and vertical rule, which is what a
// table's strokes are — and triangles for every other shape. A glyph carries
// its font, its id and its position as Typst shaped and placed it, so gift
// draws it through its own glyph atlas without shaping anything again. That
// works because [Engine.AddFont] loads the same bytes into gift's text stack:
// a glyph id means the same glyph on both sides only if both read one file.
//
// Every glyph also carries the byte offset in the source of the text it was
// shaped from, or -1 for text that was not written there — numbering, page
// numbers, text from a show rule. [Page.SourceAt] and [Document.CaretAt]
// map between the page and the source with it, which is what editing on the
// page needs: a click becomes a source offset, a source offset a caret.
//
// [Engine.PDF] exports the last document that compiled, with Typst's own
// PDF writer.
//
// # Not yet
//
// Images, data files and packages are not supplied to Typst, so documents
// cannot use them. Gradients and tilings are drawn grey, clips are ignored,
// rotated text is drawn upright, filled curves are triangulated as fans,
// which is right only for convex outlines. Every page crosses the boundary
// on every compile, where only the changed ones would have to. The caret
// heuristics know nothing of bidirectional text.
package typst

//! gifttypst: Typst behind a C ABI for gift.
//!
//! gift builds without cgo and opens this library at run time with purego, the
//! way it opens libturbojpeg. Everything crossing the boundary is a pointer and
//! a length: Go hands in UTF-8 and font bytes, Rust hands back one buffer per
//! call, encoded little endian (see `encode`), which Go copies and returns with
//! `gt_free`. No Rust type and no Go pointer outlives a call on the other side.
//!
//! The engine keeps its `World` between compilations, and Typst's memoization
//! (comemo) is process wide, so a compile after a small edit reuses everything
//! the edit did not touch. That is the whole point of keeping an engine alive
//! instead of running the command line tool.

use std::collections::HashMap;
use std::ffi::c_char;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::ptr;

use typst::diag::{FileError, FileResult, Severity, SourceDiagnostic};
use typst::foundations::{Bytes, Datetime, Duration};
use typst::layout::{Abs, Frame, FrameItem, Point, Transform};
use typst::syntax::{FileId, RootedPath, Source, Span, VirtualPath, VirtualRoot};
use typst::text::{Font, FontBook, TextItem};
use typst::utils::LazyHash;
use typst::visualize::{CurveItem, FixedStroke, Geometry, Paint, Shape};
use typst::{Library, LibraryExt, World, WorldExt};
use typst_layout::PagedDocument;

/// The stack a compilation runs on. Typst recurses through the syntax tree
/// and the layout, and the thread that calls in from Go may have far less: a
/// thread Go's runtime created gets the platform's default for pthreads.
const STACK: usize = 64 << 20;

struct GiftWorld {
    library: LazyHash<Library>,
    book: LazyHash<FontBook>,
    fonts: Vec<Font>,
    index: HashMap<Font, u32>,
    main: Source,
}

impl World for GiftWorld {
    fn library(&self) -> &LazyHash<Library> {
        &self.library
    }

    fn book(&self) -> &LazyHash<FontBook> {
        &self.book
    }

    fn main(&self) -> FileId {
        self.main.id()
    }

    fn source(&self, id: FileId) -> FileResult<Source> {
        if id == self.main.id() {
            Ok(self.main.clone())
        } else {
            Err(FileError::AccessDenied)
        }
    }

    fn file(&self, _: FileId) -> FileResult<Bytes> {
        // Images, data files and packages are the host's to supply. The
        // spike supplies none; see the package documentation on the Go side.
        Err(FileError::AccessDenied)
    }

    fn font(&self, index: usize) -> Option<Font> {
        self.fonts.get(index).cloned()
    }

    fn today(&self, _: Option<Duration>) -> Option<Datetime> {
        None
    }
}

/// Engine is what Go holds a pointer to.
pub struct Engine {
    world: GiftWorld,
    doc: Option<PagedDocument>,
}

fn main_id() -> FileId {
    RootedPath::new(VirtualRoot::Project, VirtualPath::new("/main.typ").unwrap()).intern()
}

/// gt_version returns a static, NUL terminated description of the library.
#[unsafe(no_mangle)]
pub extern "C" fn gt_version() -> *const c_char {
    c"gifttypst 0.1.0 (typst 0.15.1)".as_ptr()
}

/// gt_new creates an engine without fonts and with an empty document.
#[unsafe(no_mangle)]
pub extern "C" fn gt_new() -> *mut Engine {
    catch_unwind(|| {
        let world = GiftWorld {
            library: LazyHash::new(Library::default()),
            book: LazyHash::new(FontBook::new()),
            fonts: Vec::new(),
            index: HashMap::new(),
            main: Source::new(main_id(), String::new()),
        };
        Box::into_raw(Box::new(Engine { world, doc: None }))
    })
    .unwrap_or(ptr::null_mut())
}

/// gt_free_engine releases an engine from gt_new.
///
/// # Safety
/// e must come from gt_new and must not be used afterwards.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_free_engine(e: *mut Engine) {
    if !e.is_null() {
        drop(unsafe { Box::from_raw(e) });
    }
}

/// gt_add_font adds every face in a font file and returns how many there
/// were: 0 if the bytes hold no font, more than 1 for a collection. Faces are
/// numbered from zero in the order they were added, and text in a compiled
/// document names its font by that number.
///
/// # Safety
/// e must come from gt_new; data must point to n readable bytes.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_add_font(e: *mut Engine, data: *const u8, n: usize) -> i32 {
    let (e, data) = unsafe { (&mut *e, std::slice::from_raw_parts(data, n)) };
    catch_unwind(AssertUnwindSafe(|| {
        let first = e.world.fonts.len();
        for f in Font::iter(Bytes::new(data.to_vec())) {
            e.world.index.insert(f.clone(), e.world.fonts.len() as u32);
            e.world.fonts.push(f);
        }
        if e.world.fonts.len() > first {
            e.world.book = LazyHash::new(FontBook::from_fonts(&e.world.fonts));
        }
        (e.world.fonts.len() - first) as i32
    }))
    .unwrap_or(-1)
}

/// gt_set_source replaces the text of the document. Typst diffs the old and
/// the new text and reparses only what changed.
///
/// # Safety
/// e must come from gt_new; text must point to n bytes of UTF-8.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_set_source(e: *mut Engine, text: *const u8, n: usize) -> i32 {
    let (e, text) = unsafe { (&mut *e, std::slice::from_raw_parts(text, n)) };
    let Ok(text) = std::str::from_utf8(text) else {
        return -1;
    };
    catch_unwind(AssertUnwindSafe(|| {
        e.world.main.replace(text);
        0
    }))
    .unwrap_or(-1)
}

/// gt_compile lays the document out and returns the pages, or the errors, in
/// a buffer from `encode`. The return value is 0 for pages, 1 for errors and
/// -1 if the library itself failed.
///
/// # Safety
/// e must come from gt_new; out and n must be writable.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_compile(e: *mut Engine, out: *mut *mut u8, n: *mut usize) -> i32 {
    let e = unsafe { &mut *e };
    let result = std::thread::scope(|s| {
        std::thread::Builder::new()
            .stack_size(STACK)
            .spawn_scoped(s, || catch_unwind(AssertUnwindSafe(|| compile(e))))
            .ok()?
            .join()
            .ok()?
            .ok()
    });
    let Some((code, buf)) = result else {
        return -1;
    };
    unsafe { hand_over(buf, out, n) };
    code
}

/// gt_pdf exports the last document that compiled, or returns -1 if there is
/// none or the export failed.
///
/// # Safety
/// e must come from gt_new; out and n must be writable.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_pdf(e: *mut Engine, out: *mut *mut u8, n: *mut usize) -> i32 {
    let e = unsafe { &mut *e };
    let pdf = catch_unwind(AssertUnwindSafe(|| {
        let doc = e.doc.as_ref()?;
        typst_pdf::pdf(doc, &typst_pdf::PdfOptions::default()).ok()
    }));
    match pdf {
        Ok(Some(buf)) => {
            unsafe { hand_over(buf, out, n) };
            0
        }
        _ => -1,
    }
}

/// gt_free releases a buffer handed out by gt_compile or gt_pdf.
///
/// # Safety
/// p and n must be exactly what one of those calls wrote.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gt_free(p: *mut u8, n: usize) {
    if !p.is_null() {
        drop(unsafe { Box::from_raw(ptr::slice_from_raw_parts_mut(p, n)) });
    }
}

unsafe fn hand_over(buf: Vec<u8>, out: *mut *mut u8, n: *mut usize) {
    let b = buf.into_boxed_slice();
    unsafe {
        *n = b.len();
        *out = Box::into_raw(b) as *mut u8;
    }
}

fn compile(e: &mut Engine) -> (i32, Vec<u8>) {
    let warned = typst::compile::<PagedDocument>(&e.world);
    // Keep the memo cache from growing without bound across edits: entries
    // unused for this many compilations are dropped.
    typst::comemo::evict(10);
    match warned.output {
        Ok(doc) => {
            let buf = encode(&e.world, &doc);
            e.doc = Some(doc);
            (0, buf)
        }
        Err(errs) => (1, encode_errors(&e.world, &errs)),
    }
}

// The encoding. Little endian throughout; f32 for every length in points.
//
//   errors:   u32 count, then per error: i32 start, i32 end (byte offsets into
//             the source, -1 if unknown), u32 len, UTF-8 message
//   document: u32 pages, then per page: f32 width, f32 height, u32 items,
//             then the items, each a tag byte and its fields:
//     1 text:      u32 font, f32 size, u32 rgba, u32 glyphs, then per glyph:
//                  u32 id, f32 x, f32 y, f32 advance, i32 source offset,
//                  u32 bytes of text the glyph stands for
//     2 rect:      f32 x, f32 y, f32 w, f32 h, u32 rgba
//     3 triangles: u32 rgba, u32 vertices, then f32 x, f32 y per vertex
//     4 image:     f32 x, f32 y, f32 w, f32 h (the picture itself is not sent)

const TEXT: u8 = 1;
const RECT: u8 = 2;
const TRIS: u8 = 3;
const IMAGE: u8 = 4;

struct Out<'a> {
    world: &'a GiftWorld,
    buf: Vec<u8>,
    items: u32,
    spans: HashMap<Span, i32>,
}

impl Out<'_> {
    fn u8(&mut self, v: u8) {
        self.buf.push(v);
    }
    fn u32(&mut self, v: u32) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }
    fn i32(&mut self, v: i32) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }
    fn f32(&mut self, v: f64) {
        self.buf.extend_from_slice(&(v as f32).to_le_bytes());
    }

    /// The byte offset in the main source where the syntax node of span
    /// starts, or -1 for text that was not written there: generated text,
    /// text from a template, numbering.
    fn start(&mut self, span: Span) -> i32 {
        if span.is_detached() || span.id() != Some(self.world.main.id()) {
            return -1;
        }
        if let Some(&s) = self.spans.get(&span) {
            return s;
        }
        let s = self.world.range(span).map_or(-1, |r| r.start as i32);
        self.spans.insert(span, s);
        s
    }
}

fn encode_errors(world: &GiftWorld, errs: &[SourceDiagnostic]) -> Vec<u8> {
    let mut o = Out { world, buf: Vec::new(), items: 0, spans: HashMap::new() };
    let errs: Vec<_> = errs.iter().filter(|d| d.severity == Severity::Error).collect();
    o.u32(errs.len() as u32);
    for d in errs {
        let r = world.range(d.span);
        o.i32(r.as_ref().map_or(-1, |r| r.start as i32));
        o.i32(r.as_ref().map_or(-1, |r| r.end as i32));
        o.u32(d.message.len() as u32);
        o.buf.extend_from_slice(d.message.as_bytes());
    }
    o.buf
}

fn encode(world: &GiftWorld, doc: &PagedDocument) -> Vec<u8> {
    let mut o = Out { world, buf: Vec::new(), items: 0, spans: HashMap::new() };
    o.u32(doc.pages().len() as u32);
    for page in doc.pages() {
        let size = page.frame.size();
        o.f32(size.x.to_pt());
        o.f32(size.y.to_pt());
        let at = o.buf.len();
        o.u32(0);
        o.items = 0;
        frame(&mut o, &page.frame, Transform::identity());
        let n = o.items.to_le_bytes();
        o.buf[at..at + 4].copy_from_slice(&n);
    }
    o.buf
}

fn frame(o: &mut Out, f: &Frame, ts: Transform) {
    for (pos, item) in f.items() {
        match item {
            FrameItem::Group(g) => {
                let ts = ts
                    .pre_concat(Transform::translate(pos.x, pos.y))
                    .pre_concat(g.transform);
                frame(o, &g.frame, ts);
            }
            FrameItem::Text(t) => text(o, *pos, t, ts),
            FrameItem::Shape(s, _) => shape(o, *pos, s, ts),
            FrameItem::Image(_, size, _) => {
                let a = pos.transform(ts);
                let b = (*pos + size.to_point()).transform(ts);
                o.u8(IMAGE);
                o.f32(a.x.to_pt());
                o.f32(a.y.to_pt());
                o.f32((b.x - a.x).to_pt());
                o.f32((b.y - a.y).to_pt());
                o.items += 1;
            }
            FrameItem::Link(..) | FrameItem::Tag(_) => {}
        }
    }
}

fn rgba(p: &Paint) -> u32 {
    match p {
        Paint::Solid(c) => u32::from_be_bytes(c.to_vec4_u8()),
        // Gradients and tilings are drawn in their first colour, or grey;
        // the spike does not render them.
        _ => 0x808080ff,
    }
}

fn text(o: &mut Out, pos: Point, t: &TextItem, ts: Transform) {
    let Some(&font) = o.world.index.get(t.font.font()) else {
        return;
    };
    let size = t.size.to_pt();
    o.u8(TEXT);
    o.u32(font);
    // A uniform scale only; text under a rotation or a skew is drawn upright.
    o.f32(size * ts.sy.get());
    o.u32(rgba(&t.fill));
    o.u32(t.glyphs.len() as u32);
    let mut x = 0.0;
    for g in &t.glyphs {
        let p = Point::new(
            pos.x + Abs::pt(x + g.x_offset.get() * size),
            pos.y - Abs::pt(g.y_offset.get() * size),
        )
        .transform(ts);
        let adv = g.x_advance.get() * size;
        let start = o.start(g.span.0);
        o.u32(g.id as u32);
        o.f32(p.x.to_pt());
        o.f32(p.y.to_pt());
        o.f32(adv * ts.sx.get());
        o.i32(if start < 0 { -1 } else { start + g.span.1 as i32 });
        o.u32(g.range().len() as u32);
        x += adv;
    }
    o.items += 1;
}

fn shape(o: &mut Out, pos: Point, s: &Shape, ts: Transform) {
    let ts = ts.pre_concat(Transform::translate(pos.x, pos.y));
    let axis = ts.kx.get() == 0.0 && ts.ky.get() == 0.0;
    match &s.geometry {
        Geometry::Rect(size) => {
            let a = Point::zero().transform(ts);
            let b = size.to_point().transform(ts);
            if let Some(fill) = &s.fill {
                if axis {
                    rect(o, a, b, rgba(fill));
                } else {
                    let pts = [Point::zero(), Point::with_x(size.x), size.to_point(), Point::with_y(size.y)];
                    fan(o, &pts.map(|p| p.transform(ts)), rgba(fill));
                }
            }
            if let Some(st) = &s.stroke {
                let pts = [Point::zero(), Point::with_x(size.x), size.to_point(), Point::with_y(size.y), Point::zero()];
                polyline(o, &pts.map(|p| p.transform(ts)), st);
            }
        }
        Geometry::Line(to) => {
            if let Some(st) = &s.stroke {
                polyline(o, &[Point::zero().transform(ts), to.transform(ts)], st);
            }
        }
        Geometry::Curve(c) => {
            let mut paths: Vec<Vec<Point>> = Vec::new();
            let mut cur = Point::zero();
            for item in c.0.iter() {
                match *item {
                    CurveItem::Move(p) => {
                        paths.push(vec![p.transform(ts)]);
                        cur = p;
                    }
                    CurveItem::Line(p) => {
                        if paths.is_empty() {
                            paths.push(vec![cur.transform(ts)]);
                        }
                        paths.last_mut().unwrap().push(p.transform(ts));
                        cur = p;
                    }
                    CurveItem::Cubic(c1, c2, p) => {
                        if paths.is_empty() {
                            paths.push(vec![cur.transform(ts)]);
                        }
                        let path = paths.last_mut().unwrap();
                        const STEPS: usize = 12;
                        for i in 1..=STEPS {
                            let t = i as f64 / STEPS as f64;
                            let u = 1.0 - t;
                            let q = cur * (u * u * u) + c1 * (3.0 * u * u * t) + c2 * (3.0 * u * t * t) + p * (t * t * t);
                            path.push(q.transform(ts));
                        }
                        cur = p;
                    }
                    CurveItem::Close => {
                        if let Some(path) = paths.last_mut() {
                            if let Some(&first) = path.first() {
                                path.push(first);
                            }
                        }
                    }
                }
            }
            for path in &paths {
                if let Some(fill) = &s.fill {
                    // A fan is right for convex outlines — circles, rounded
                    // rectangles, the shapes tables and boxes make — and
                    // wrong for others, which the spike accepts.
                    fan(o, path, rgba(fill));
                }
                if let Some(st) = &s.stroke {
                    polyline(o, path, st);
                }
            }
        }
    }
}

fn rect(o: &mut Out, a: Point, b: Point, rgba: u32) {
    o.u8(RECT);
    o.f32(a.x.min(b.x).to_pt());
    o.f32(a.y.min(b.y).to_pt());
    o.f32((b.x - a.x).abs().to_pt());
    o.f32((b.y - a.y).abs().to_pt());
    o.u32(rgba);
    o.items += 1;
}

fn fan(o: &mut Out, pts: &[Point], rgba: u32) {
    if pts.len() < 3 {
        return;
    }
    o.u8(TRIS);
    o.u32(rgba);
    o.u32(((pts.len() - 2) * 3) as u32);
    for i in 1..pts.len() - 1 {
        for p in [pts[0], pts[i], pts[i + 1]] {
            o.f32(p.x.to_pt());
            o.f32(p.y.to_pt());
        }
    }
    o.items += 1;
}

/// polyline strokes the segments of pts. An axis aligned segment — every rule
/// of a table — becomes a rectangle, anything else a quad of two triangles.
fn polyline(o: &mut Out, pts: &[Point], st: &FixedStroke) {
    let w = st.thickness.to_pt();
    let c = rgba(&st.paint);
    for seg in pts.windows(2) {
        let (a, b) = (seg[0], seg[1]);
        let (dx, dy) = ((b.x - a.x).to_pt(), (b.y - a.y).to_pt());
        let len = (dx * dx + dy * dy).sqrt();
        if len == 0.0 {
            continue;
        }
        let (nx, ny) = (-dy / len * w / 2.0, dx / len * w / 2.0);
        if dx == 0.0 || dy == 0.0 {
            rect(
                o,
                Point::new(a.x - Abs::pt(nx.abs()), a.y - Abs::pt(ny.abs())),
                Point::new(b.x + Abs::pt(nx.abs()), b.y + Abs::pt(ny.abs())),
                c,
            );
            continue;
        }
        let n = Point::new(Abs::pt(nx), Abs::pt(ny));
        let q = [a + n, b + n, b - n, a - n];
        fan(o, &q, c);
    }
}

// Package imageviewer shows pictures as large and as interactive as the screen
// allows: a picture that zooms and pans in place, and a full screen viewer
// that pages through a catalogue, in the manner of iOS's Photos.
//
//   - [Zoomable] is one picture in a view of its own size, to pinch, double
//     tap and wheel into, that stays out of the way of a page it scrolls in.
//   - [Fullscreen] is the view of a [Viewer]: the pictures of an
//     [asset.Collection] side by side, swiped through, zoomed, dragged down to
//     close, with a card that slides up from below. It grows out of the
//     thumbnail the user tapped and shrinks back into it, wherever the
//     [Origin] — a [ui.Gallery], say — reports that thumbnail to be.
//
// # Feel
//
// The physics are UIKit's where Apple says what they are and measured
// reconstructions where it does not; package ui/motion records each number and
// its source. Pinched past its limits a picture follows the fingers with the
// rubber band of a UIScrollView, applied to the logarithm of the zoom, and
// springs back when they lift; panned past its edge it rubber bands and
// bounces; flicked it decelerates at UIScrollView's normal rate. A zoomed
// picture panned on past its edge hands the movement on to the next page, as
// Photos does with its nested scroll views. The gestures themselves are
// package ui/gesture, and nothing here recognises anything on its own.
//
// # Loading in stages
//
// A picture is shown in up to three stages, each through the one image
// pipeline of [ui.SetImagePipeline] and drawn as soon as it is there:
//
//  1. a preview of [PreviewSize] — the size a gallery decodes its tiles at,
//     so that a picture opened from a gallery is in the cache already and
//     appears in the first frame of the transition. One that is not is
//     decoded at that size first, which a JPEG decoder that scales, such as
//     asset/turbojpeg, does from the DC coefficient of every 8 by 8 block
//     alone: the macroblocks, and the cheapest picture there is;
//  2. the size of the screen, which is what a picture at rest needs;
//  3. while it is zoomed, a larger decode, up to [MaxDetail].
//
// Stages 2 and 3 come from the pipeline's ladder, so it needs rungs as large as
// the screen: a pipeline that stops at 512 shows a full screen picture at 512.
// An application with a viewer configures [asset.Config.Sizes] accordingly,
// for example 128, 256, 512, 1024, 2048 and 4096, and keeps the large rungs out
// of the disk cache with [asset.DiskCacheConfig.MaxSize].
package imageviewer

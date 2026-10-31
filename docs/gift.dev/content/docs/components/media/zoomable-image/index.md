---
title: Zoomable Image
---

A zoomable image is a picture that can be zoomed and panned where it is: pinched with two fingers, double tapped,
wheeled with the control or command key held, and moved with the arrow keys and plus, minus and zero once it has
the focus. Pinched past its limits it follows the fingers with growing resistance and springs back when they lift,
as a `UIScrollView` with `bouncesZoom` does; panned past its edge it rubber bands and bounces; flicked it glides on
and comes to rest.

{{< gallery-shot alt="A gradient picture in a rounded grey well" >}}

{{< gallery-code >}}

## Usage

`imageviewer.Zoomable(source)` takes any `asset.Source` — a file, a URL, an entry of a catalogue — and loads it
through the [image pipeline](../../../concepts/images/), like `ui.Image`.

```go
imageviewer.Zoomable(asset.File("/media/photos/IMG_0042.jpg")).
	MaxZoom(6).            // relative to the fit; default: twice the picture's pixels, at least 3
	DoubleTapZoom(2.5).    // default: the zoom that fills the view, at least 2
	Wheel(imageviewer.WheelZooms).
	Frame(600, 400)
```

Not zoomed, the view leaves a one finger drag and the plain wheel to its ancestors. A zoomable image inside a
[scroll view](../../layout/scroll-view/) therefore scrolls with the page until it is pinched, and pans once it is
zoomed. `Wheel` decides what the wheel does: `WheelWithModifier`, the default, zooms only with control or command,
`WheelZooms` always, `WheelOff` never.

## Loading in stages

The picture arrives in up to three stages and each one is drawn as soon as it is there: a preview of
`imageviewer.PreviewSize`, the size the view is shown at, and while it is zoomed a larger decode of up to
`imageviewer.MaxDetail` pixels. With `asset/turbojpeg` registered, the preview of a camera JPEG is decoded from the
DC coefficients of its 8 by 8 blocks alone. The pipeline needs rungs as large as the view; see
[Image Viewer](../image-viewer/#the-pipeline).

## Pinch on a Raspberry Pi

A pinch needs two fingers, and a touch screen on Raspberry Pi OS reaches the window as a one finger mouse. The
Ebitengine backend therefore reads the panel directly; see [Gestures](../../../concepts/gestures/#touch-screens-on-linux).

## API

- [`imageviewer.Zoomable`](https://pkg.go.dev/github.com/worldiety/gift/ui/imageviewer#Zoomable)
- [`imageviewer.ZoomableView`](https://pkg.go.dev/github.com/worldiety/gift/ui/imageviewer#ZoomableView)

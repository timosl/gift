---
title: Media
weight: 7
---

Media components show pictures as large as the screen allows, and play moving pictures and sound. The pictures
are package [`ui/imageviewer`](https://github.com/worldiety/gift/tree/main/ui/imageviewer), on top of the
[image pipeline](../../concepts/images/) and the [gestures](../../concepts/gestures/). The video player lives in
package [`ui/videoplayer`](https://github.com/worldiety/gift/tree/main/ui/videoplayer) and decodes through package
[`video`](https://github.com/worldiety/gift/tree/main/video), which loads the FFmpeg libraries of the system at run
time, without cgo.

- [Image Viewer](image-viewer/): the pictures of a catalogue full screen, opened out of their thumbnails, to swipe,
  zoom and drag away, with a card of details.
- [Map](map/): a map after MapKit, from self hosted tiles, with photographs that cluster.
- [Zoomable Image](zoomable-image/): one picture to pinch, double tap and wheel into, in place.

- [Video Player](video-player/): a video with sound and touch controls for play, pause, position, volume and the
  audio track, or the bare picture without them.

---
title: Media
weight: 7
---

Media components play moving pictures and sound. They live in package
[`ui/videoplayer`](https://github.com/worldiety/gift/tree/main/ui/videoplayer) and decode through package
[`video`](https://github.com/worldiety/gift/tree/main/video), which loads the FFmpeg libraries of the system at run
time, without cgo.

- [Video Player](video-player/): a video with sound and touch controls for play, pause, position, volume and the
  audio track, or the bare picture without them.

---
title: Video
weight: 5
---

This example plays a video with sound full window, with the touch controls of the
[video player](../../components/media/video-player/). It is the program to try on a Raspberry Pi with a five or
seven inch touch panel: the window below has the size of a five inch panel, 800 by 480 pixels.

{{< gallery-shot alt="A video of a rabbit filling the window, with round play, back and forward buttons, a volume slider, a headphones button and a position bar" >}}

The player is the root view and not the content of a `ui.Window`, and the backend runs with `DirectToScreen` and
`DrawOnDemand`. That is what lets a Raspberry Pi 5 show full HD at 60 frames a second: a video at its own size is
converted from YUV in the pass that draws it, and nothing is drawn under it.

## Run it

You need the FFmpeg libraries at run time. Raspberry Pi OS has them; on macOS, run `brew install ffmpeg`. From a
checkout of the [repository](https://github.com/worldiety/gift):

```bash
go run ./cmd/example-video                          # the three second clip in video/testdata
go run ./cmd/example-video -file film.mp4 -hw       # your own, HEVC in hardware on a Raspberry Pi 5
go run ./cmd/example-video -width 1024 -height 600  # a seven inch panel
```

`-fullscreen` fills the screen, `-autohide 0` keeps the controls, `-loop=false` stops at the end, and `-stats`
prints the frame rate and how the video was drawn every five seconds.

## What to try

- Tap the picture to show or hide the controls. While the video plays, they hide by themselves after three seconds.
- Drag along the position bar. The picture follows your finger, and the video goes on where you lift it.
- Open the headphones menu and switch between the English stereo and the German 5.1 track of the clip.

## Code

{{< example-code >}}

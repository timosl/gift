# Screenshots

These PNGs are framebuffer captures of the running examples, taken through
`giftauto` at the default window sizes (Retina density 2 on macOS).

Start one example at a time from the repository root:

```sh
go run -tags giftauto ./cmd/example-kitchensink
go run -tags giftauto ./cmd/example-gallery
go run -tags giftauto ./cmd/example-effects -quality=full
```

In another terminal, wait for the window and its images to finish loading,
then capture it, replacing the filename for each example:

```sh
curl -fsS 'http://127.0.0.1:7391/screenshot?settle=8' \
  -o docs/screenshots/kitchensink.png
```

The kitchen sink screenshot uses the Home tab and the dark theme:

```sh
curl -fsS -X POST http://127.0.0.1:7391/theme -d '{"mode":"dark"}'
```

`video.png` was taken on a Raspberry Pi 5 (Raspberry Pi OS Trixie, labwc), with the
window at the size of a five inch panel, the clip paused and the audio track menu open:

```sh
go run -tags giftauto ./cmd/example-video -width 800 -height 480 -autohide 0
# tap the play button, then the headphones button, through POST /input
curl -fsS 'http://127.0.0.1:7391/screenshot?settle=3' -o docs/screenshots/video.png
```

`photos.png` was taken on a Raspberry Pi 5 at 1920x1080 and scaled to 1200 pixels. The
viewer was opened and its card swiped up on the virtual touch screen of `gift-touchinject`,
through the same path a real panel takes:

```sh
example-photos -fullscreen -workers 2        # built with -tags giftauto
sudo gift-touchinject -hz 50 tap:1500,276 wait:1500 swipe:960,900,960,500,300 wait:1500
curl -fsS http://127.0.0.1:7391/screenshot -o photos.png
```

No offscreen reconstruction or separate screenshot application is involved.

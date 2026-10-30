# Test media

`bbb.mp4` is three seconds of *Big Buck Bunny*, (c) copyright 2008, Blender Foundation,
www.bigbuckbunny.org, licensed under the [Creative Commons Attribution 3.0](https://creativecommons.org/licenses/by/3.0/)
license. It was cut from `bbb_sunflower_1080p_60fps_normal.mp4` and re-encoded:

```sh
ffmpeg -ss 61 -t 3 -i bbb_sunflower_1080p_60fps_normal.mp4 -map 0:v:0 -map 0:a:0 -map 0:a:1 \
  -vf "fps=25,scale=640:360" -c:v libx264 -preset slow -crf 27 -profile:v main -pix_fmt yuv420p \
  -c:a aac -b:a:0 64k -ac:a:0 2 -b:a:1 96k -ac:a:1 6 \
  -metadata:s:a:0 language=eng -metadata:s:a:1 language=deu \
  -disposition:a:0 default -disposition:a:1 0 -movflags +faststart bbb.mp4
```

It has an English stereo and a German 5.1 track, so that the tests can switch tracks and the 5.1 track goes
through the downmix.

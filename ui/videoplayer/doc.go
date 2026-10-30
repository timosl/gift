// Package videoplayer shows a [video.Player]: its picture, its sound and
// controls a finger can operate.
//
// [New] is the whole player, laid out like the player of iOS and sized for a
// five inch touch panel; [Surface] is the picture alone, for an application
// that brings its own controls. Both take a player the application opened
// and keeps:
//
//	p, err := video.Open("film.mp4", video.Options{HW: true})
//	if err != nil {
//		return ui.Text(err.Error())
//	}
//	p.Play()
//	return videoplayer.New(p)
//
// The views draw through the backend's [render.Videos] and play through its
// [render.AudioOutput]. Without a backend, in a gifttest harness, the player
// runs on the wall clock and the picture is black.
//
// # On a Raspberry Pi
//
// A Raspberry Pi 5 shows full HD at 60 frames a second when the window runs
// with backend.Config.DirectToScreen and nothing is drawn under the picture:
// make the player the root view rather than the content of a [ui.Window].
// See the project plan, section 25, for the measurements.
//
// It is a package of its own, like ui/filepicker, so that package ui does not
// depend on package video and its FFmpeg binding.
package videoplayer

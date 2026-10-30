package videoplayer

import (
	"strconv"
	"strings"
	"time"

	"github.com/worldiety/gift/video/ffmpeg"
)

// clock formats d as m:ss, or h:mm:ss from an hour on, rounded down to the
// second: what a player shows, where 0:59 turns into 1:00 when the minute is
// full and not half a second earlier.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d / time.Second)
	h, m := s/3600, s/60%60
	s %= 60
	var b strings.Builder
	if h > 0 {
		b.WriteString(strconv.Itoa(h))
		b.WriteByte(':')
		if m < 10 {
			b.WriteByte('0')
		}
	}
	b.WriteString(strconv.Itoa(m))
	b.WriteByte(':')
	if s < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.Itoa(s))
	return b.String()
}

// remaining formats what is left as "-m:ss", rounded up so that the two
// labels add up to the duration.
func remaining(pos, dur time.Duration) string {
	left := dur - pos
	if left < 0 {
		left = 0
	}
	return "-" + clock(left+time.Second-1)
}

// languages names the ISO 639-2 codes files carry most, in their own
// language, which is how a person who speaks one finds it in a list.
var languages = map[string]string{
	"eng": "English", "deu": "Deutsch", "ger": "Deutsch", "fra": "Français", "fre": "Français",
	"spa": "Español", "ita": "Italiano", "nld": "Nederlands", "dut": "Nederlands",
	"por": "Português", "pol": "Polski", "ces": "Čeština", "cze": "Čeština", "dan": "Dansk",
	"swe": "Svenska", "nor": "Norsk", "fin": "Suomi", "tur": "Türkçe", "rus": "Русский",
	"ukr": "Українська", "ell": "Ελληνικά", "gre": "Ελληνικά", "jpn": "日本語",
	"zho": "中文", "chi": "中文", "kor": "한국어", "ara": "العربية", "heb": "עברית",
}

// channels names a channel count the way a menu does.
func channels(n int) string {
	switch n {
	case 1:
		return "Mono"
	case 2:
		return "Stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	if n > 0 {
		return strconv.Itoa(n) + " channels"
	}
	return ""
}

// trackName is the line a track gets in the track menu: its title if the
// file gives one, otherwise its language and channels, otherwise its number.
func trackName(t ffmpeg.AudioTrack, i int) string {
	if t.Title != "" {
		return t.Title
	}
	var parts []string
	if name, ok := languages[t.Language]; ok {
		parts = append(parts, name)
	} else if t.Language != "" && t.Language != "und" {
		parts = append(parts, t.Language)
	}
	if c := channels(t.Channels); c != "" {
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return "Track " + strconv.Itoa(i+1)
	}
	return strings.Join(parts, " · ")
}

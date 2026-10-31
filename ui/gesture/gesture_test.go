package gesture_test

import (
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/gesture"
)

func TestMain(m *testing.M) { gifttest.Main(m) }

// pad is a 400 by 300 surface at the origin with the gestures on it.
func pad(t *testing.T, gs ...gesture.Gesture) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{
		View: ui.ZStack(gesture.On(ui.Box().Frame(400, 300), gs...).Key("pad")).Align(geom.TopLeading),
		Size: geom.Sz(600, 400),
	})
	h.Settle()
	return h
}

func TestATapFiresOnce(t *testing.T) {
	var got []gesture.TapValue
	h := pad(t, gesture.Tap().OnEnded(func(v gesture.TapValue) { got = append(got, v) }))
	h.TapAt(geom.Pt(100, 100))
	if len(got) != 1 || got[0].Location != geom.Pt(100, 100) || got[0].Device != gift.PointerTouch {
		t.Fatalf("taps = %+v", got)
	}
	// A press that wanders further than the slop is no tap.
	h.FingerDown(1, geom.Pt(100, 100))
	h.FingerMove(1, geom.Pt(130, 100))
	h.FingerUp(1, geom.Pt(130, 100))
	if len(got) != 1 {
		t.Fatalf("a drag counted as a tap: %+v", got)
	}
}

// TestASingleTapWaitsForADoubleTapToFail pins the require-to-fail rule.
func TestASingleTapWaitsForADoubleTapToFail(t *testing.T) {
	singles, doubles := 0, 0
	h := pad(t,
		gesture.Tap().Count(2).OnEnded(func(gesture.TapValue) { doubles++ }),
		gesture.Tap().OnEnded(func(gesture.TapValue) { singles++ }),
	)
	h.TapAt(geom.Pt(50, 50))
	h.Advance(100 * time.Millisecond)
	h.TapAt(geom.Pt(52, 50))
	h.Advance(time.Second)
	if doubles != 1 || singles != 0 {
		t.Fatalf("a double tap fired %d doubles and %d singles, want 1 and 0", doubles, singles)
	}
	h.TapAt(geom.Pt(50, 50))
	if singles != 0 {
		t.Fatal("the single tap fired before the double tap could fail")
	}
	h.AdvanceTicks(30, gifttest.FrameInterval)
	if singles != 1 || doubles != 1 {
		t.Fatalf("after the interval: %d singles, %d doubles, want 1 and 1", singles, doubles)
	}
}

func TestALongPressTakesThePressFromTheTap(t *testing.T) {
	taps, longs := 0, 0
	h := pad(t,
		gesture.Tap().OnEnded(func(gesture.TapValue) { taps++ }),
		gesture.LongPress().OnEnded(func(gesture.LongPressValue) { longs++ }),
	)
	h.FingerDown(1, geom.Pt(80, 80))
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if longs != 1 {
		t.Fatalf("long presses = %d after 660 ms", longs)
	}
	h.FingerUp(1, geom.Pt(80, 80))
	if taps != 0 {
		t.Fatal("the release of a long press counted as a tap")
	}
}

func TestDragReportsTranslationAndVelocity(t *testing.T) {
	var began, ended *gesture.DragValue
	changes := 0
	h := pad(t, gesture.Drag().
		OnChanged(func(v gesture.DragValue) {
			changes++
			if v.Phase == gesture.Began {
				began = &v
			}
		}).
		OnEnded(func(v gesture.DragValue) { ended = &v }))
	h.Fling(geom.Pt(50, 150), geom.Pt(350, 150), 300*time.Millisecond)
	if began == nil || ended == nil {
		t.Fatalf("began %v, ended %v", began, ended)
	}
	if began.StartLocation != geom.Pt(50, 150) {
		t.Errorf("StartLocation = %v", began.StartLocation)
	}
	if ended.Translation != geom.Pt(300, 0) {
		t.Errorf("Translation at the end = %v, want (300, 0)", ended.Translation)
	}
	// 300 px in 0.3 s.
	if v := ended.Velocity.X; v < 900 || v > 1100 {
		t.Errorf("Velocity = %v, want about 1000 px/s", ended.Velocity)
	}
	if p := ended.PredictedEndTranslation.X; p < 700 || p > 900 {
		t.Errorf("PredictedEndTranslation = %v, want about 300 + 499", ended.PredictedEndTranslation)
	}
	if changes < 5 {
		t.Errorf("%d changes for a drag of 18 moves", changes)
	}
}

func TestADirectionalDragFailsInTheOtherDirection(t *testing.T) {
	horizontal, vertical := 0, 0
	h := pad(t,
		gesture.Drag().Axis(gesture.Horizontal).OnChanged(func(gesture.DragValue) { horizontal++ }),
		gesture.Drag().Axis(gesture.Vertical).OnChanged(func(gesture.DragValue) { vertical++ }),
	)
	h.Fling(geom.Pt(200, 50), geom.Pt(210, 250), 200*time.Millisecond)
	if horizontal != 0 || vertical == 0 {
		t.Fatalf("a mostly vertical drag ran the horizontal one %d times and the vertical one %d times", horizontal, vertical)
	}
}

func TestPinchReportsMagnificationAroundTheCentroid(t *testing.T) {
	var last gesture.MagnifyValue
	ended := false
	h := pad(t, gesture.Magnify().
		OnChanged(func(v gesture.MagnifyValue) { last = v }).
		OnEnded(func(v gesture.MagnifyValue) { last, ended = v, true }))
	h.Pinch(geom.Pt(200, 150), 100, 250, 250*time.Millisecond)
	if !ended {
		t.Fatal("the pinch did not end")
	}
	if math.Abs(last.Magnification-2.5) > 0.01 {
		t.Errorf("Magnification = %v, want 2.5", last.Magnification)
	}
	if last.StartAnchor != geom.Pt(200, 150) {
		t.Errorf("StartAnchor = %v", last.StartAnchor)
	}
	if last.Velocity <= 0 {
		t.Errorf("Velocity = %v for a spreading pinch", last.Velocity)
	}
}

// TestAPinchAndADragShareTheFingers: the drag follows the centroid, so a pinch
// that moves pans, and a finger that joins does not make the drag jump.
func TestAPinchAndADragShareTheFingers(t *testing.T) {
	var drag gesture.DragValue
	mag := 1.0
	h := pad(t,
		gesture.Drag().OnChanged(func(v gesture.DragValue) { drag = v }),
		gesture.Magnify().OnChanged(func(v gesture.MagnifyValue) { mag = v.Magnification }),
	)
	h.FingerDown(1, geom.Pt(100, 100))
	h.FingerMove(1, geom.Pt(120, 100))
	before := drag.Translation
	// The second finger lands 100 px to the right: the centroid jumps by
	// 50, the translation must not.
	h.FingerDown(2, geom.Pt(220, 100))
	h.FingerMove(2, geom.Pt(230, 100))
	if d := drag.Translation.X - before.X; d < 4 || d > 6 {
		t.Errorf("translation moved by %v when the second finger moved 10, want 5 (the centroid)", d)
	}
	if mag <= 1 {
		t.Errorf("the spread did not magnify: %v", mag)
	}
	h.FingerUp(2, geom.Pt(230, 100))
	h.FingerUp(1, geom.Pt(120, 100))
}

func TestHoverFollowsTheMouseAndSaysWhenItLeaves(t *testing.T) {
	var got []gesture.HoverValue
	h := pad(t, gesture.Hover().OnChanged(func(v gesture.HoverValue) { got = append(got, v) }))
	h.MoveTo(geom.Pt(100, 100))
	h.MoveTo(geom.Pt(120, 110))
	// A cursor reported again where it rests is not a movement.
	h.MoveTo(geom.Pt(120, 110))
	if len(got) != 2 {
		t.Fatalf("%d hover values for two movements and a rest", len(got))
	}
	h.MoveTo(geom.Pt(500, 350))
	if len(got) < 3 || !got[0].Inside || got[len(got)-1].Inside {
		t.Fatalf("hover = %+v, want inside moves and a final leave", got)
	}
}

func TestAnUnusedWheelBubbles(t *testing.T) {
	used := 0
	h := pad(t, gesture.Wheel().OnChanged(func(v gesture.WheelValue) bool {
		if v.Mods.Has(gift.ModControl) {
			used++
			return true
		}
		return false
	}))
	h.Wheel(geom.Pt(100, 100), geom.Pt(0, 1))
	if used != 0 {
		t.Fatal("an unmodified wheel was used")
	}
}

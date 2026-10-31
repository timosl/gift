package maps

import (
	"testing"

	"github.com/worldiety/gift/geom"
)

func TestALabelGivesWayToOneThatWasPlacedFirst(t *testing.T) {
	n := &node{f: &frame{}}
	f := n.f
	f.resetBoxes(800, 600)
	f.addBox(geom.Rc(700, 580, 800, 600))
	place := func(text string, r geom.Rect) bool {
		c := geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		if n.collides(r, text, c) {
			return false
		}
		f.labels = append(f.labels, placed{text: text, c: c})
		f.addBox(r)
		return true
	}
	if !place("Oldenburg", geom.Rc(100, 100, 200, 120)) {
		t.Fatal("the first label was refused")
	}
	if place("Lefferseck", geom.Rc(150, 110, 260, 130)) {
		t.Error("an overlapping label was placed")
	}
	if place("Haaren", geom.Rc(400, 100, 450, 115)) == false {
		t.Error("a free label was refused")
	}
	if place("Haaren", geom.Rc(500, 200, 550, 215)) {
		t.Error("the same name was placed again close by")
	}
	if !place("Haaren", geom.Rc(400, 500, 450, 515)) {
		t.Error("the same name far away was refused")
	}
	if place("Damm", geom.Rc(720, 585, 760, 598)) {
		t.Error("a label was placed under the attribution")
	}
}

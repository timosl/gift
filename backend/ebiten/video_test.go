package ebiten

import (
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// videoFixture presents one grey NV12 picture of w by h and returns its id.
func videoFixture(t *testing.T, w, h int) (*Renderer, *capture, render.ImageID) {
	t.Helper()
	r, c := newHeadlessRenderer(t)
	r.BeginFrame(geom.Sz(1920, 1080))
	stride := (w + 7) / 8 * 8
	pix := make([]byte, stride*(h+(h+1)/2))
	for i := range pix {
		pix[i] = 128
	}
	hd, ok := r.Videos().Present(render.ImageHandle{}, render.VideoFrame{
		Pix: pix, Stride: stride, Width: w, Height: h, Layout: render.VideoNV12, Matrix: render.BT709,
	})
	if !ok {
		t.Fatal("Present refused a well formed picture")
	}
	id, ok := r.Videos().Resolve(hd)
	if !ok {
		t.Fatal("a presented picture does not resolve")
	}
	return r, c, id
}

func submitVideo(r *Renderer, op render.Op) {
	var l render.List
	l.Reset()
	l.Add(op)
	r.Submit(&l)
	r.EndFrame()
}

func TestVideoShadersCompile(t *testing.T) {
	for name, src := range map[string][]byte{"video.kage": videoConvertSrc, "videodirect.kage": videoDirectSrc} {
		if _, err := eb.NewShader(src); err != nil {
			t.Errorf("%s does not compile: %v", name, err)
		}
	}
}

// TestVideoAtItsOwnSizeIsConvertedInThePassThatDrawsIt pins the rule of
// VideoCache: a picture shown one texel per device pixel takes the direct
// shader and no converted texture.
func TestVideoAtItsOwnSizeIsConvertedInThePassThatDrawsIt(t *testing.T) {
	r, c, id := videoFixture(t, 640, 360)
	submitVideo(r, render.Op{Kind: render.OpVideo, Bounds: geom.Rc(10, 20, 650, 380), Color: render.RGB(255, 255, 255), Image: id})
	if len(c.mats) != 1 || c.mats[0] != MaterialVideo {
		t.Fatalf("materials = %v, want one video batch", c.mats)
	}
	if s := r.Stats(); s.VideoOps != 1 || s.VideoDirectOps != 1 {
		t.Errorf("VideoOps %d, VideoDirectOps %d, want 1 and 1", s.VideoOps, s.VideoDirectOps)
	}
	if r.videos.conversions != 0 {
		t.Errorf("%d conversions for a direct picture", r.videos.conversions)
	}
	// The source coordinates are the picture's own pixels; the shader reads
	// its position from them.
	if v := c.verts[2]; v.SrcX != 640 || v.SrcY != 360 {
		t.Errorf("far corner samples (%v, %v), want (640, 360)", v.SrcX, v.SrcY)
	}
}

func TestScaledVideoIsDrawnFromAConvertedTexture(t *testing.T) {
	r, c, id := videoFixture(t, 640, 360)
	op := render.Op{Kind: render.OpVideo, Bounds: geom.Rc(0, 0, 1920, 1080), Color: render.RGB(255, 255, 255), Image: id}
	submitVideo(r, op)
	if len(c.mats) != 1 || c.mats[0] != MaterialImage {
		t.Fatalf("materials = %v, want one image batch", c.mats)
	}
	// A second frame of the same picture converts nothing again.
	r.BeginFrame(geom.Sz(1920, 1080))
	submitVideo(r, op)
	if r.videos.conversions != 1 {
		t.Errorf("%d conversions for one picture drawn twice, want 1", r.videos.conversions)
	}
	if s := r.Stats(); s.VideoDirectOps != 0 || s.VideoOps != 2 {
		t.Errorf("VideoOps %d, VideoDirectOps %d, want 2 and 0", s.VideoOps, s.VideoDirectOps)
	}
}

func TestRoundedOrTintedVideoIsNotDirect(t *testing.T) {
	for name, op := range map[string]render.Op{
		"rounded": {Kind: render.OpVideo, Bounds: geom.Rc(0, 0, 640, 360), Color: render.RGB(255, 255, 255), CornerRadius: 12},
		"tinted":  {Kind: render.OpVideo, Bounds: geom.Rc(0, 0, 640, 360), Color: render.RGBA(255, 255, 255, 128)},
	} {
		t.Run(name, func(t *testing.T) {
			r, c, id := videoFixture(t, 640, 360)
			op.Image = id
			submitVideo(r, op)
			if len(c.mats) != 1 || c.mats[0] == MaterialVideo {
				t.Fatalf("materials = %v, want the converted texture", c.mats)
			}
		})
	}
}

func TestPresentRejectsMalformedFramesAndReleaseFrees(t *testing.T) {
	r, _, _ := videoFixture(t, 64, 36)
	v := r.Videos()
	if _, ok := v.Present(render.ImageHandle{}, render.VideoFrame{Width: 64, Height: 36, Stride: 64, Layout: render.VideoNV12, Pix: make([]byte, 10)}); ok {
		t.Error("a short buffer was accepted")
	}
	if _, ok := v.Present(render.ImageHandle{}, render.VideoFrame{Width: 64, Height: 36, Stride: 64, Layout: render.VideoNV12}); ok {
		t.Error("a new resource without pixels was accepted")
	}
	hd, _ := v.Present(render.ImageHandle{}, render.VideoFrame{Width: 64, Height: 36, Stride: 64, Layout: render.VideoNV12, Pix: make([]byte, 64*54)})
	v.Release(hd)
	if _, ok := v.Resolve(hd); ok {
		t.Error("a released handle still resolves")
	}
}

// TestALayerWithAVideoIsDrawnAgainForEveryPicture pins the stamp: the
// operation of a playing video does not change, its picture does.
func TestALayerWithAVideoIsDrawnAgainForEveryPicture(t *testing.T) {
	r, _, id := videoFixture(t, 64, 36)
	before := r.videos.stamp(id)
	hd := render.ImageHandle{ID: id, Gen: r.videos.recs[id].gen}
	if _, ok := r.Videos().Present(hd, render.VideoFrame{Width: 64, Height: 36, Stride: 64, Layout: render.VideoNV12, Pix: make([]byte, 64*54)}); !ok {
		t.Fatal("Present refused the next picture")
	}
	if r.videos.stamp(id) == before {
		t.Error("a new picture left the layer stamp unchanged")
	}
}

func TestVideoMatrixMapsReferenceColours(t *testing.T) {
	// Limited range BT.709: Y=16 is black, Y=235 white, chroma 128 grey.
	r, g, b := videoMatrix(render.BT709, false)
	at := func(row [4]float32, y, u, v float32) float32 {
		return row[0]*y/255 + row[1]*u/255 + row[2]*v/255 + row[3]
	}
	for _, c := range []struct {
		y, want float32
	}{{16, 0}, {235, 1}} {
		for i, row := range [][4]float32{r, g, b} {
			if got := at(row, c.y, 128, 128); got < c.want-0.01 || got > c.want+0.01 {
				t.Errorf("channel %d of Y=%v is %v, want %v", i, c.y, got, c.want)
			}
		}
	}
}

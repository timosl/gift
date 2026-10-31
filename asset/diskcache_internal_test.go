package asset

import "testing"

func TestLargeRungsStayOffTheDisk(t *testing.T) {
	d := newDiskCache(DiskCacheConfig{Dir: t.TempDir(), MaxSize: 512})
	small := Key{ID: "a", Revision: "r", Size: 512}
	large := Key{ID: "a", Revision: "r", Size: 2048}
	th := &Thumbnail{pix: make([]byte, 4*4*4), w: 4, h: 4, stride: 16, bytes: 64}
	if err := d.put(small, th); err != nil {
		t.Fatal(err)
	}
	if err := d.put(large, th); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := d.get(small); !ok {
		t.Error("the 512 rung was not kept")
	}
	if _, _, _, ok := d.get(large); ok {
		t.Error("the 2048 rung was written although MaxSize is 512")
	}
}

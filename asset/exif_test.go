package asset_test

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/internal/exifwrite"
)

func TestTheLocationAndTimeOfAPhotographAreRead(t *testing.T) {
	taken := time.Date(2026, 9, 12, 14, 30, 5, 0, time.UTC)
	for _, c := range []struct{ lat, lon float64 }{
		{53.1435, 8.2146}, {-33.8688, 151.2093}, {64.1466, -21.9426},
	} {
		raw, err := exifwrite.JPEG(image.NewRGBA(image.Rect(0, 0, 16, 16)), c.lat, c.lon, taken)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "p.jpg")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		x, err := asset.ReadExif(context.Background(), asset.File(path))
		if err != nil {
			t.Fatal(err)
		}
		if !x.HasLocation || math.Abs(x.Location.Lat-c.lat) > 1e-6 || math.Abs(x.Location.Lon-c.lon) > 1e-6 {
			t.Errorf("location %v (%v), want %v, %v", x.Location, x.HasLocation, c.lat, c.lon)
		}
		if !x.Taken.Equal(taken) {
			t.Errorf("taken %v, want %v", x.Taken, taken)
		}
		// The picture still decodes.
		if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
			t.Errorf("the JPEG with EXIF does not decode: %v", err)
		}
	}
}

func TestAPictureWithoutExifHasNoLocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.png")
	if err := os.WriteFile(path, pngBytes(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	x, err := asset.ReadExif(context.Background(), asset.File(path))
	if err != nil || x.HasLocation || !x.Taken.IsZero() {
		t.Fatalf("%+v, %v", x, err)
	}
}

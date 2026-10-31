package maps_test

import (
	"testing"

	"github.com/worldiety/gift/gifttest"
)

// TestMain runs the tests on Ebitengine's main thread when the GPU half is
// built in; see gifttest.Main.
func TestMain(m *testing.M) { gifttest.Main(m) }

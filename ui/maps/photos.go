package maps

import (
	"context"
	"runtime"
	"sync"

	"github.com/worldiety/gift/asset"
)

// LocatePhotos reads where and when each source was taken, with
// [asset.ReadExif] on a few goroutines, and returns a point for each that
// says where: the points a map of photographs is made of. The ID of a point
// is the ID of its source. Sources without a location are left out; a
// source that cannot be read is left out too, and the first such error is
// returned with the points that could.
func LocatePhotos(ctx context.Context, srcs []asset.Source) ([]Point, error) {
	type result struct {
		p  Point
		ok bool
	}
	res := make([]result, len(srcs))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		next     = make(chan int)
	)
	workers := min(len(srcs), max(1, runtime.GOMAXPROCS(0)/2))
	for range workers {
		wg.Go(func() {
			for i := range next {
				x, err := asset.ReadExif(ctx, srcs[i])
				if err != nil {
					once.Do(func() { firstErr = err })
					continue
				}
				if x.HasLocation {
					res[i] = result{Point{
						ID: string(srcs[i].Metadata().ID), At: x.Location, Time: x.Taken, Photo: srcs[i],
					}, true}
				}
			}
		})
	}
	for i := range srcs {
		if ctx.Err() != nil {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()
	out := make([]Point, 0, len(srcs))
	for _, r := range res {
		if r.ok {
			out = append(out, r.p)
		}
	}
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	return out, firstErr
}

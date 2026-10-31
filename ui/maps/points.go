package maps

import "github.com/worldiety/gift"

type pointLayer struct{}

func (n *node) pointViews(kids []gift.View) []gift.View { return kids }

func (n *node) layoutPoints(ctx *gift.LayoutContext, v projection) {}

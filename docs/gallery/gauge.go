package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "gauge", Group: "basic", Width: 440, Height: 180}, gaugeDemo)
}

func gaugeDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return ui.HStack(
		ui.Gauge(21.5, 10, 30).CurrentValueLabel("21.5°").MinMaxLabels("10", "30").Size(96),
		ui.Gauge(0.62, 0, 1).Capacity(true).CurrentValueLabel("62 %").
			Tint(ui.RGB(255, 149, 0)).Size(96),
		ui.Gauge(84, 0, 120).CurrentValueLabel("84").MinMaxLabels("0", "120").
			Tint(ui.RGB(255, 59, 48)).Size(96).LineWidth(12),
	).Gap(32).Align(geom.Center)
	// snippet:end
}

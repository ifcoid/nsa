package modules

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"nsa/internal/model"
)

type chartItem struct {
	Label string
	Value int
}

// svgBarChart membuat SVG bar chart sederhana (rasio 16:9, tanpa dependency eksternal).
func svgBarChart(title string, items []chartItem) string {
	const W, H = 800, 450
	const padL, padR, padT, padB = 50, 20, 50, 95
	plotW := W - padL - padR
	plotH := H - padT - padB
	maxV := 1
	for _, it := range items {
		if it.Value > maxV {
			maxV = it.Value
		}
	}
	n := len(items)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" font-family="sans-serif">`, W, H)
	b.WriteString(`<rect width="100%" height="100%" fill="#0f172a"/>`)
	fmt.Fprintf(&b, `<text x="%d" y="30" fill="#e2e8f0" font-size="18" font-weight="bold">%s</text>`, padL, html.EscapeString(title))
	baseY := padT + plotH
	if n == 0 {
		fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#94a3b8" font-size="14">(tidak ada data)</text></svg>`, padL, H/2)
		return b.String()
	}
	fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#334155"/>`, padL, baseY, W-padR, baseY)
	gap := 10
	bw := (plotW - gap*n) / n
	if bw < 4 {
		bw = 4
	}
	for i, it := range items {
		bh := int(float64(it.Value) / float64(maxV) * float64(plotH))
		x := padL + gap/2 + i*(bw+gap)
		y := baseY - bh
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#3b82f6" rx="2"/>`, x, y, bw, bh)
		fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#cbd5e1" font-size="11" text-anchor="middle">%d</text>`, x+bw/2, y-4, it.Value)
		lbl := it.Label
		if len(lbl) > 18 {
			lbl = lbl[:18]
		}
		cx := x + bw/2
		fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#94a3b8" font-size="10" text-anchor="end" transform="rotate(-40 %d %d)">%s</text>`, cx, baseY+14, cx, baseY+14, html.EscapeString(lbl))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// figureFromCounts membuat model.Figure dari map counts. sortByKey=true mengurutkan
// berdasarkan label (mis. tahun); selain itu berdasarkan nilai (desc).
func figureFromCounts(name, title string, counts map[string]int, sortByKey bool) model.Figure {
	items := make([]chartItem, 0, len(counts))
	for k, v := range counts {
		items = append(items, chartItem{Label: k, Value: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if sortByKey {
			return items[i].Label < items[j].Label
		}
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		return items[i].Label < items[j].Label
	})
	if len(items) > 15 {
		items = items[:15]
	}
	return model.Figure{Name: name, SVG: svgBarChart(title, items)}
}

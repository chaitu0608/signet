package profile

import (
	"fmt"
	"strings"

	"server/internal/store"
)

const accentLime = "#a3e635"
const accentSky = "#7dd3fc"

func shortAddr(addr string) string {
	if len(addr) > 12 {
		return addr[:8] + "…" + addr[len(addr)-4:]
	}
	return addr
}

func sigilGradient(addr string) string {
	seed := 0
	if len(addr) > 10 {
		fmt.Sscanf(addr[2:10], "%x", &seed)
	}
	hue := seed % 360
	return fmt.Sprintf("linear-gradient(135deg,hsl(%d,70%%,45%%),hsl(%d,70%%,55%%))", hue, (hue+80)%360)
}

func renderBadgeSVG(prof *store.DevProfile, addr string) string {
	short := shortAddr(addr)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="320" height="80">
<rect width="320" height="80" rx="8" fill="#0f172a"/>
<circle cx="40" cy="40" r="18" fill="url(#g)"/>
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
<stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/>
</linearGradient></defs>
<text x="16" y="28" fill="%s" font-family="monospace" font-size="13" font-weight="bold">Signet Verified</text>
<text x="66" y="48" fill="#e2e8f0" font-family="monospace" font-size="11">%s</text>
<text x="66" y="66" fill="#94a3b8" font-family="monospace" font-size="10">%d rep · %d commits</text>
</svg>`, accentLime, accentSky, accentLime, short, prof.TotalScore, prof.CommitCount)
}

func renderOGSVG(prof *store.DevProfile, addr string) string {
	short := shortAddr(addr)
	repos := "—"
	if len(prof.TopRepos) > 0 {
		parts := make([]string, 0, 3)
		for i, r := range prof.TopRepos {
			if i >= 3 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", r.Repo, r.Score))
		}
		repos = strings.Join(parts, " · ")
	}
	delta := prof.WeeklyDelta
	deltaStr := fmt.Sprintf("%d", delta)
	if delta > 0 {
		deltaStr = "+" + deltaStr
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630">
<rect width="1200" height="630" fill="#060608"/>
<rect x="0" y="0" width="1200" height="8" fill="%s"/>
<text x="60" y="90" fill="%s" font-family="monospace" font-size="48" font-weight="bold">SIGNET</text>
<text x="60" y="130" fill="#64748b" font-family="monospace" font-size="22">Proof of code.</text>
<circle cx="150" cy="280" r="70" fill="url(#og)"/>
<defs><linearGradient id="og" x1="0" y1="0" x2="1" y2="1">
<stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/>
</linearGradient></defs>
<text x="260" y="260" fill="#e2e8f0" font-family="monospace" font-size="36">%s</text>
<text x="260" y="310" fill="#64748b" font-family="monospace" font-size="22">%d rep · %d commits · 7d %s</text>
<text x="60" y="420" fill="#94a3b8" font-family="monospace" font-size="24">Top repos: %s</text>
<text x="60" y="580" fill="#475569" font-family="monospace" font-size="18">Verified onchain — base-sepolia.easscan.org</text>
</svg>`, accentLime, accentLime, accentLime, accentSky, short, prof.TotalScore, prof.CommitCount, deltaStr, repos)
}

func sparklineBars(buckets []store.DayBucket, width, height int) string {
	if len(buckets) == 0 {
		return ""
	}
	maxScore := 1
	for _, b := range buckets {
		if b.Score > maxScore {
			maxScore = b.Score
		}
	}
	n := len(buckets)
	if n > 7 {
		buckets = buckets[len(buckets)-7:]
		n = 7
	}
	barW := width / (n + 1)
	var bars strings.Builder
	for i, b := range buckets {
		h := 4
		if b.Score > 0 {
			h = 4 + (b.Score * (height - 8) / maxScore)
		}
		x := (i + 1) * barW
		y := height - h
		bars.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="1" fill="%s" opacity="0.8"/>`, x, y, barW-2, h, accentLime))
	}
	return bars.String()
}

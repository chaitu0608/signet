package profile

import (
	"fmt"
	"net/http"
	"strings"

	"server/internal/store"
)

// HandleEmbed serves an embeddable badge page at /embed/{address}.
func HandleEmbed(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addr := strings.TrimPrefix(r.URL.Path, "/embed/")
		addr = strings.Trim(addr, "/")
		if addr == "" || !strings.HasPrefix(strings.ToLower(addr), "0x") {
			http.Error(w, "address required", http.StatusBadRequest)
			return
		}

		variant := r.URL.Query().Get("v")
		if variant == "" {
			variant = "standard"
		}

		prof, err := st.DevProfile(r.Context(), addr, 1000)
		if err != nil || prof == nil {
			prof = &store.DevProfile{Address: addr}
		}
		sparkline, _ := st.Timeseries(r.Context(), addr, 7)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "frame-ancestors *")
		w.Header().Set("Cache-Control", "public, max-age=60")

		short := shortAddr(addr)
		grad := sigilGradient(addr)
		topRepo := "—"
		if len(prof.TopRepos) > 0 {
			topRepo = prof.TopRepos[0].Repo
		}

		switch variant {
		case "compact":
			fmt.Fprintf(w, compactEmbedHTML(), grad, addr, short, prof.TotalScore, addr)
		case "detailed":
			spark := sparklineBars(sparkline, 120, 24)
			fmt.Fprintf(w, detailedEmbedHTML(), grad, addr, short, prof.TotalScore, prof.CommitCount, topRepo, spark, addr)
		default:
			fmt.Fprintf(w, standardEmbedHTML(), grad, addr, short, prof.TotalScore, prof.CommitCount, addr)
		}
	}
}

func compactEmbedHTML() string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"/>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:system-ui,sans-serif}
  a{text-decoration:none;color:inherit;display:flex;align-items:center;gap:8px;padding:8px 12px;
    border-radius:6px;border:1px solid #334155;background:#0f172a;color:#e2e8f0;min-height:60px;width:200px}
  .dot{width:28px;height:28px;border-radius:50%%;background:%s;flex-shrink:0}
  .accent{color:#a3e635;font-size:9px;font-weight:700;letter-spacing:.08em}
  .addr{font-family:ui-monospace,monospace;font-size:10px;color:#94a3b8}
  .stat{font-size:16px;font-weight:700;color:#a3e635}
</style></head><body>
<a href="/dev/%s" target="_blank" rel="noopener">
  <div class="dot"></div>
  <div><div class="accent">SIGNET</div><div class="addr">%s</div><div class="stat">%d rep</div></div>
</a>
<script>
try{fetch("https://api.ensideas.com/ens/resolve/%s").then(r=>r.json()).then(d=>{
  if(d.name){var el=document.querySelector(".addr");if(el)el.textContent=d.name;}
}).catch(()=>{})}catch(e){}
</script>
</body></html>`
}

func standardEmbedHTML() string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"/>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:system-ui,sans-serif}
  a{text-decoration:none;color:inherit;display:flex;align-items:center;gap:12px;padding:12px 16px;
    border-radius:8px;border:1px solid #334155;background:#0f172a;color:#e2e8f0;min-height:80px;width:320px}
  .dot{width:40px;height:40px;border-radius:50%%;background:%s;flex-shrink:0}
  .accent{color:#a3e635;font-size:11px;font-weight:600;letter-spacing:.05em}
  .addr{font-family:ui-monospace,monospace;font-size:12px}
  .stat{font-size:20px;font-weight:700}
  .muted{color:#94a3b8;font-size:11px;margin-top:2px}
</style></head><body>
<a href="/dev/%s" target="_blank" rel="noopener">
  <div class="dot"></div>
  <div>
    <div class="accent">SIGNET VERIFIED</div>
    <div class="addr">%s</div>
    <div class="stat">%d rep</div>
    <div class="muted">%d attested commits · proof of code</div>
  </div>
</a>
<script>
try{fetch("https://api.ensideas.com/ens/resolve/%s").then(r=>r.json()).then(d=>{
  if(d.name){var el=document.querySelector(".addr");if(el)el.textContent=d.name;}
}).catch(()=>{})}catch(e){}
</script>
</body></html>`
}

func detailedEmbedHTML() string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"/>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:system-ui,sans-serif}
  a{text-decoration:none;color:inherit;display:block;padding:14px 16px;border-radius:8px;border:1px solid #334155;
    background:#0f172a;color:#e2e8f0;width:440px;min-height:140px}
  .row{display:flex;align-items:center;gap:12px}
  .dot{width:44px;height:44px;border-radius:50%%;background:%s;flex-shrink:0}
  .accent{color:#a3e635;font-size:11px;font-weight:600;letter-spacing:.05em}
  .addr{font-family:ui-monospace,monospace;font-size:12px;color:#94a3b8}
  .stat{font-size:22px;font-weight:700}
  .muted{color:#64748b;font-size:10px;margin-top:4px}
  .spark{margin-top:10px}
</style></head><body>
<a href="/dev/%s" target="_blank" rel="noopener">
  <div class="row">
    <div class="dot"></div>
    <div>
      <div class="accent">SIGNET VERIFIED</div>
      <div class="addr">%s</div>
      <div class="stat">%d rep</div>
      <div class="muted">%d commits · top: %s</div>
    </div>
  </div>
  <svg class="spark" width="120" height="24" xmlns="http://www.w3.org/2000/svg">%s</svg>
</a>
<script>
try{fetch("https://api.ensideas.com/ens/resolve/%s").then(r=>r.json()).then(d=>{
  if(d.name){var el=document.querySelector(".addr");if(el)el.textContent=d.name;}
}).catch(()=>{})}catch(e){}
</script>
</body></html>`
}

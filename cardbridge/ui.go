package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// The bridge's own page on http://127.0.0.1:17777: pairing, status, the
// machines' drivers, start-with-the-computer and the recent log. It listens on
// the computer itself only (never the shop network); a random key in the page
// guards every change against other web pages.
type ui struct {
	cfg  *configStore
	w    *worker
	log  *ringLog
	key  string
	port int
	tpl  *template.Template
}

func newUI(cfg *configStore, w *worker, l *ringLog) *ui {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &ui{cfg: cfg, w: w, log: l, key: hex.EncodeToString(b), port: cfg.get().Port, tpl: template.Must(template.New("p").Parse(pageHTML))}
}

// okHost: only http://127.0.0.1:port / localhost:port (stops DNS rebinding).
func (u *ui) okHost(r *http.Request) bool {
	h := r.Host
	p := strconv.Itoa(u.port)
	return h == "127.0.0.1:"+p || h == "localhost:"+p || h == "[::1]:"+p
}

func (u *ui) okPost(r *http.Request) bool {
	if r.Method != http.MethodPost || !u.okHost(r) {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "null" {
		ou, err := url.Parse(o)
		if err != nil || !u.okHost(&http.Request{Host: ou.Host}) {
			return false
		}
	}
	return subtle.ConstantTimeCompare([]byte(r.FormValue("k")), []byte(u.key)) == 1
}

// validServer: https, or http on this computer / a private test server.
func validServer(s string) (string, bool) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	p, err := url.Parse(s)
	if err != nil || p.Host == "" || p.User != nil || p.RawQuery != "" {
		return "", false
	}
	if p.Scheme == "https" {
		return s, true
	}
	host := p.Hostname()
	if p.Scheme == "http" && (host == "localhost" || host == "127.0.0.1") {
		return s, true
	}
	return "", false
}

// trustedServer: StartERP's own servers (others get a warning on the page).
func trustedServer(s string) bool {
	p, err := url.Parse(s)
	if err != nil {
		return false
	}
	h := p.Hostname()
	return strings.HasSuffix(h, ".gulfunionozone.com") || h == "starterp.org" || strings.HasSuffix(h, ".starterp.org") || h == "localhost" || h == "127.0.0.1"
}

type pageData struct {
	Key, Version, Server, Code, Name, Store, Message, Error string
	Paired, Connected, Autostart, Untrusted                 bool
	LastPoll, LastError, Busy                               string
	Drivers                                                 []Driver
	Log                                                     []string
	OS                                                      string
}

func (u *ui) page(w http.ResponseWriter, r *http.Request, msg, errMsg string) {
	c := u.cfg.get()
	st := u.w.snapshot()
	d := pageData{Key: u.key, Version: Version, Server: c.Server, Name: c.Name, Store: c.StoreName,
		Paired: c.paired(), Connected: st.Connected, Busy: st.Busy, LastError: st.LastError,
		Message: msg, Error: errMsg, Autostart: autostartInstalled(), Log: u.log.tail(30), OS: osName()}
	if !st.LastPoll.IsZero() {
		d.LastPoll = st.LastPoll.Format("15:04:05")
	}
	q := r.URL.Query()
	if v := q.Get("server"); v != "" && !d.Paired {
		if s, ok := validServer(v); ok {
			d.Server = s
		}
	}
	d.Code = q.Get("code")
	d.Untrusted = !trustedServer(d.Server)
	for _, id := range driverIDs() {
		d.Drivers = append(d.Drivers, driverByID(id))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_ = u.tpl.Execute(w, d)
}

func (u *ui) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !u.okHost(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/pair" {
			http.NotFound(w, r)
			return
		}
		u.page(w, r, "", "")
	})
	mux.HandleFunc("/status.json", func(w http.ResponseWriter, r *http.Request) {
		if !u.okHost(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		c := u.cfg.get()
		st := u.w.snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": Version, "paired": c.paired(), "connected": st.Connected,
			"store": c.StoreName, "drivers": driverIDs()})
	})
	mux.HandleFunc("/do/pair", func(w http.ResponseWriter, r *http.Request) {
		if !u.okPost(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		server, ok := validServer(r.FormValue("server"))
		if !ok {
			u.page(w, r, "", "The StartERP server address must start with https://")
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = u.cfg.get().Name
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		a, err := newClient(server, "").pair(ctx, strings.TrimSpace(r.FormValue("code")), name)
		if err != nil {
			u.page(w, r, "", err.Error())
			return
		}
		if err := u.cfg.update(func(c *Config) {
			c.Server, c.Token, c.BridgeID, c.StoreID, c.StoreName, c.Name = server, a.Token, a.BridgeID, a.StoreID, a.StoreName, name
		}); err != nil {
			u.page(w, r, "", "Could not save the settings: "+err.Error())
			return
		}
		u.log.add("Paired with " + a.StoreName)
		u.w.wake()
		http.Redirect(w, r, "/?paired=1", http.StatusSeeOther)
	})
	mux.HandleFunc("/do/unpair", func(w http.ResponseWriter, r *http.Request) {
		if !u.okPost(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = u.cfg.update(func(c *Config) { c.Token, c.BridgeID, c.StoreID, c.StoreName = "", "", "", "" })
		u.log.add("Unpaired on this computer")
		u.w.wake()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("/do/autostart", func(w http.ResponseWriter, r *http.Request) {
		if !u.okPost(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var err error
		if r.FormValue("on") == "1" {
			exe, e := os.Executable()
			if e != nil {
				err = e
			} else {
				err = installAutostart(exe)
			}
		} else {
			err = uninstallAutostart()
		}
		if err != nil {
			u.page(w, r, "", "Could not change start-up: "+err.Error())
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	return mux
}

// listen on 127.0.0.1 only.
func (u *ui) listen() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(u.port))
}

const pageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>StartERP Card Bridge</title>
<style>
:root{--bg:#f6f7f9;--card:#fff;--text:#14181f;--muted:#5b6474;--line:#dfe3ea;--brand:#0f766e;--ok:#15803d;--bad:#b91c1c;--warn:#a16207}
@media (prefers-color-scheme:dark){:root{--bg:#0f1318;--card:#171c23;--text:#e8ebf0;--muted:#9aa4b2;--line:#2a313b;--brand:#2dd4bf;--ok:#4ade80;--bad:#f87171;--warn:#facc15}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,"Noto Sans Arabic",sans-serif}
main{max-width:760px;margin:0 auto;padding:24px 16px}h1{font-size:22px;margin:0 0 4px}h2{font-size:16px;margin:0 0 8px}
.sub{color:var(--muted);margin:0 0 18px}.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px;margin:0 0 14px}
.ar{direction:rtl;text-align:right;color:var(--muted);font-size:14px;margin:2px 0 0}
label{display:block;font-weight:600;margin:10px 0 4px}input{width:100%;padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:var(--bg);color:var(--text);font:inherit}
button{margin-top:12px;padding:10px 16px;border:0;border-radius:8px;background:var(--brand);color:#fff;font:inherit;font-weight:600;cursor:pointer}
button.ghost{background:transparent;color:var(--text);border:1px solid var(--line)}
.ok{color:var(--ok)}.bad{color:var(--bad)}.warn{color:var(--warn)}.muted{color:var(--muted)}
.dot{display:inline-block;width:10px;height:10px;border-radius:50%;margin-inline-end:6px;background:var(--bad)}.dot.on{background:var(--ok)}
ol{padding-inline-start:20px;margin:6px 0}pre{white-space:pre-wrap;font:12px/1.5 ui-monospace,Menlo,Consolas,monospace;margin:0;max-height:260px;overflow:auto}
.msg{padding:10px 12px;border-radius:8px;margin:0 0 14px;border:1px solid var(--line)}.msg.bad{border-color:var(--bad)}
code{font:13px ui-monospace,Menlo,Consolas,monospace}
</style></head><body><main>
<h1>StartERP Card Bridge</h1>
<p class="sub">Connects this computer's card machines to StartERP · version {{.Version}} · {{.OS}}<span class="ar">جسر البطاقات: يربط أجهزة البطاقات بهذا الكمبيوتر مع StartERP</span></p>
{{if .Error}}<p class="msg bad" role="alert">{{.Error}}</p>{{end}}
{{if .Paired}}
<section class="card"><h2><span class="dot {{if .Connected}}on{{end}}"></span>{{if .Connected}}Connected to StartERP{{else}}Not connected{{end}}</h2>
<p>Store: <b>{{.Store}}</b><br>This computer: <b>{{.Name}}</b><br><span class="muted">Server: <code>{{.Server}}</code>{{if .LastPoll}} · last contact {{.LastPoll}}{{end}}</span></p>
{{if .Busy}}<p class="warn">A payment is on {{.Busy}} now.</p>{{end}}
{{if .LastError}}<p class="bad">{{.LastError}}</p>{{end}}
<p class="ar">{{if .Connected}}متصل بـ StartERP{{else}}غير متصل{{end}}</p>
<form method="post" action="/do/unpair"><input type="hidden" name="k" value="{{.Key}}"><button class="ghost" type="submit">Unpair this computer</button></form>
</section>
{{else}}
<section class="card"><h2>Pair this computer with your store</h2>
<ol><li>In StartERP open <b>Settings → Card machines</b> and press <b>Pair a computer</b>.</li><li>Type the 8-character code shown there and press <b>Pair</b>.</li></ol>
<p class="ar">في StartERP افتح الإعدادات ← أجهزة البطاقات واضغط «ربط كمبيوتر»، ثم اكتب الرمز المكوّن من 8 خانات واضغط «ربط».</p>
<form method="post" action="/do/pair"><input type="hidden" name="k" value="{{.Key}}">
<label for="code">Pairing code</label><input id="code" name="code" value="{{.Code}}" autocomplete="off" placeholder="XXXX-XXXX" required maxlength="12" autofocus>
<label for="name">Name of this computer</label><input id="name" name="name" value="{{.Name}}" maxlength="60">
<label for="server">StartERP server</label><input id="server" name="server" value="{{.Server}}" required>
{{if .Untrusted}}<p class="warn">This is not a StartERP server address. Only pair with the address shown in StartERP's Settings → Card machines.</p>{{end}}
<button type="submit">Pair</button></form></section>
{{end}}
<section class="card"><h2>Start with the computer</h2>
<p>{{if .Autostart}}<span class="ok">On:</span> the Card Bridge starts by itself when you sign in to this computer.{{else}}<span class="warn">Off:</span> turn it on so the tills can reach the card machine after a restart.{{end}}</p>
<form method="post" action="/do/autostart"><input type="hidden" name="k" value="{{.Key}}"><input type="hidden" name="on" value="{{if .Autostart}}0{{else}}1{{end}}"><button class="{{if .Autostart}}ghost{{end}}" type="submit">{{if .Autostart}}Turn off{{else}}Turn on{{end}}</button></form>
</section>
<section class="card"><h2>Card machine drivers in this version</h2><ol>{{range .Drivers}}<li><b>{{.Title}}</b> <span class="muted">({{.ID}})</span></li>{{end}}</ol>
<p class="muted">Each card machine's settings (IP address, port, cable) are entered in StartERP: Settings → Card machines → Edit.</p></section>
<section class="card"><h2>Recent activity</h2><pre>{{range .Log}}{{.}}
{{else}}Nothing yet.{{end}}</pre></section>
<p class="muted">Keep this computer on and connected to the internet while the shop is open. This page is only reachable from this computer.</p>
</main></body></html>`

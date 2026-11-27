package qbittorrent

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

// infoDict is the bencoded info dictionary of the fixture torrent.
var infoDict = "d6:lengthi5e4:name1:x12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "e"

var torrentFile = []byte("d8:announce3:foo4:info" + infoDict + "e")

func fixtureHash() string {
	sum := sha1.Sum([]byte(infoDict))
	return hex.EncodeToString(sum[:])
}

const magnetHex = "0123456789abcdef0123456789abcdef01234567"

// fakeQB is a stateful in-memory qBittorrent Web API v2.
type fakeQB struct {
	srv        *httptest.Server
	version    string
	cookieName string
	bypass     bool // authentication bypassed: no cookie needed or issued
	user, pass string

	mu         sync.Mutex
	sessions   map[string]bool
	torrents   map[string]map[string]any
	categories map[string]string
	calls      []string
	forms      []map[string]string // last add form fields
	noQueueing bool
	logins     int
	prefs      map[string]any // app/preferences; nil answers 404
	dlLimit    int64          // bytes/s
	upLimit    int64
	alt        bool
	trackers   map[string][]map[string]any
}

func newFake(t *testing.T, version string) *fakeQB {
	t.Helper()
	f := &fakeQB{version: version, cookieName: "SID", user: "admin", pass: "pw", sessions: map[string]bool{},
		torrents: map[string]map[string]any{}, categories: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeQB) log(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeQB) called(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (f *fakeQB) expire() {
	f.mu.Lock()
	f.sessions = map[string]bool{}
	f.mu.Unlock()
}

func (f *fakeQB) authorized(r *http.Request) bool {
	if f.bypass {
		return true
	}
	ck, err := r.Cookie(f.cookieName)
	f.mu.Lock()
	defer f.mu.Unlock()
	return err == nil && f.sessions[ck.Value]
}

func (f *fakeQB) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
	f.log(path)
	if path == "auth/login" {
		f.login(w, r)
		return
	}
	if r.Header.Get("Referer") == "" || !f.authorized(r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	f.route(w, r, path)
}

func (f *fakeQB) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	f.logins++
	ok := r.PostForm.Get("username") == f.user && r.PostForm.Get("password") == f.pass
	f.mu.Unlock()
	if !ok {
		_, _ = w.Write([]byte("Fails."))
		return
	}
	if !f.bypass {
		f.mu.Lock()
		f.sessions["tok"+string(rune('a'+f.logins))] = true
		tok := "tok" + string(rune('a'+f.logins))
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: f.cookieName, Value: tok, Path: "/"})
	}
	_, _ = w.Write([]byte("Ok."))
}

func (f *fakeQB) route(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "app/version":
		_, _ = w.Write([]byte(f.version))
	case "app/preferences":
		f.preferences(w)
	case "torrents/info":
		f.info(w, r)
	case "torrents/add":
		f.add(w, r)
	case "torrents/delete":
		f.remove(w, r)
	case "torrents/files":
		f.files(w, r)
	case "torrents/categories":
		f.listCategories(w)
	case "torrents/createCategory", "torrents/editCategory":
		_ = r.ParseForm()
		f.mu.Lock()
		f.categories[r.PostForm.Get("category")] = r.PostForm.Get("savePath")
		f.mu.Unlock()
	case "torrents/pause", "torrents/resume":
		f.stateChange(w, r, f.version, "v4")
	case "torrents/stop", "torrents/start":
		f.stateChange(w, r, f.version, "v5")
	case "torrents/topPrio", "torrents/bottomPrio", "torrents/increasePrio", "torrents/decreasePrio":
		if f.noQueueing {
			w.WriteHeader(http.StatusConflict)
		}
	default:
		f.transfer(w, r, path)
	}
}

// transfer serves the global transfer, sync and per-torrent maintenance calls.
func (f *fakeQB) transfer(w http.ResponseWriter, r *http.Request, path string) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch path {
	case "transfer/downloadLimit":
		_, _ = w.Write([]byte(strconv.FormatInt(f.dlLimit, 10)))
	case "transfer/uploadLimit":
		_, _ = w.Write([]byte(strconv.FormatInt(f.upLimit, 10)))
	case "transfer/setDownloadLimit":
		f.dlLimit, _ = strconv.ParseInt(r.PostForm.Get("limit"), 10, 64)
	case "transfer/setUploadLimit":
		f.upLimit, _ = strconv.ParseInt(r.PostForm.Get("limit"), 10, 64)
	case "transfer/speedLimitsMode":
		_, _ = w.Write([]byte(map[bool]string{true: "1", false: "0"}[f.alt]))
	case "transfer/toggleSpeedLimitsMode":
		f.alt = !f.alt
	case "torrents/recheck", "torrents/reannounce", "torrents/setForceStart":
		f.calls = append(f.calls, path+"="+r.PostForm.Get("hashes")+r.PostForm.Get("value"))
	case "torrents/setCategory":
		f.setCategory(w, r)
	case "torrents/trackers":
		_ = json.NewEncoder(w).Encode(f.trackers[r.URL.Query().Get("hash")])
	case "sync/maindata":
		f.mainData(w)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeQB) setCategory(w http.ResponseWriter, r *http.Request) {
	cat := r.PostForm.Get("category")
	if _, ok := f.categories[cat]; !ok && cat != "" {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if ti := f.torrents[r.PostForm.Get("hashes")]; ti != nil {
		ti["category"] = cat
	}
}

func (f *fakeQB) mainData(w http.ResponseWriter) {
	torrents := map[string]any{}
	for h, ti := range f.torrents {
		torrents[h] = map[string]any{"state": ti["state"]}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"torrents": torrents, "server_state": map[string]any{
		"dl_info_speed": 2048, "up_info_speed": 1024, "free_space_on_disk": 5 << 30,
		"dl_rate_limit": f.dlLimit, "up_rate_limit": f.upLimit, "use_alt_speed_limits": f.alt}})
}

func (f *fakeQB) preferences(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.prefs == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(f.prefs)
}

func (f *fakeQB) stateChange(w http.ResponseWriter, r *http.Request, version, api string) {
	major := "v4"
	if strings.HasPrefix(version, "v5") {
		major = "v5"
	}
	if major != api {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeQB) info(w http.ResponseWriter, r *http.Request) {
	cat, hashes := r.URL.Query().Get("category"), r.URL.Query().Get("hashes")
	out := []map[string]any{}
	f.mu.Lock()
	for h, ti := range f.torrents {
		if (cat == "" || ti["category"] == cat) && (hashes == "" || hashes == h) {
			out = append(out, ti)
		}
	}
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(out)
}

func (f *fakeQB) add(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	form := map[string]string{}
	for k, v := range r.MultipartForm.Value {
		form[k] = v[0]
	}
	hash := f.hashOf(r, form)
	f.mu.Lock()
	f.forms = append(f.forms, form)
	if _, dup := f.torrents[hash]; dup || hash == "" {
		f.mu.Unlock()
		_, _ = w.Write([]byte("Fails."))
		return
	}
	f.torrents[hash] = map[string]any{"hash": hash, "name": "added", "category": form["category"], "state": "downloading",
		"save_path": form["savepath"], "size": 100}
	f.mu.Unlock()
	_, _ = w.Write([]byte("Ok."))
}

func (f *fakeQB) hashOf(r *http.Request, form map[string]string) string {
	if urls := form["urls"]; urls != "" {
		h, _ := magnetHash(urls)
		return h
	}
	files := r.MultipartForm.File["torrents"]
	if len(files) == 0 {
		return ""
	}
	fh, _ := files[0].Open()
	defer fh.Close()
	buf := make([]byte, 4096)
	n, _ := fh.Read(buf)
	h, _ := torrentHash(buf[:n])
	return h
}

func (f *fakeQB) remove(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	delete(f.torrents, r.PostForm.Get("hashes"))
	f.calls = append(f.calls, "deleteFiles="+r.PostForm.Get("deleteFiles"))
	f.mu.Unlock()
}

func (f *fakeQB) files(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	ti := f.torrents[r.URL.Query().Get("hash")]
	f.mu.Unlock()
	if ti == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if fl, ok := ti["files"].(string); ok && fl == "garbage" {
		_, _ = w.Write([]byte("{"))
		return
	}
	files, _ := ti["files"].([]map[string]any)
	if files == nil {
		files = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(files)
}

func (f *fakeQB) listCategories(w http.ResponseWriter) {
	out := map[string]map[string]string{}
	f.mu.Lock()
	for name, sp := range f.categories {
		out[name] = map[string]string{"name": name, "savePath": sp}
	}
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(out)
}

func (f *fakeQB) put(hash string, ti map[string]any) {
	f.mu.Lock()
	ti["hash"] = hash
	f.torrents[hash] = ti
	f.mu.Unlock()
}

func newClient(t *testing.T, f *fakeQB, extra plugin.Settings) *Client {
	t.Helper()
	s := plugin.Settings{"url": f.srv.URL + "/", "username": f.user, "password": f.pass}
	for k, v := range extra {
		s[k] = v
	}
	p, err := New(plugin.Instance{Key: "qb", Name: "qB", PluginID: ID, Settings: s, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Client)
}

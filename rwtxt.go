package rwtxt

import (
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/websocket"
	"github.com/schollz/documentsimilarity"
	"github.com/schollz/rwtxt/pkg/db"
	"github.com/schollz/rwtxt/pkg/utils"
)

const DefaultBind = ":8152"

type RWTxt struct {
	Config           Config
	viewEditTemplate *template.Template
	mainTemplate     *template.Template
	loginTemplate    *template.Template
	listTemplate     *template.Template
	prismTemplate    []string
	fs               *db.FileSystem
	wsupgrader       websocket.Upgrader
}

type Config struct {
	Bind            string // interface:port to listen on, defaults to DefaultBind.
	Private         bool
	ResizeWidth     int
	ResizeOnUpload  bool
	ResizeOnRequest bool
	OrderByCreated  bool
}

func New(fs *db.FileSystem, configUser ...Config) (*RWTxt, error) {
	config := Config{
		Bind: ":8152",
	}
	if len(configUser) > 0 {
		config = configUser[0]
	}
	rwt := &RWTxt{
		Config: config,
		fs:     fs,
		wsupgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	funcMap := template.FuncMap{
		"replace": replace,
	}

	var err error
	headerFooter := []string{"assets/header.html", "assets/footer.html"}

	b, err := Asset("assets/viewedit.html")
	if err != nil {
		return nil, err
	}
	rwt.viewEditTemplate = template.Must(template.New("viewedit").Parse(string(b)))

	err = templateAssets(headerFooter, rwt.viewEditTemplate)

	b, err = Asset("assets/main.html")
	if err != nil {
		return nil, err
	}

	rwt.mainTemplate = template.Must(template.New("main").Funcs(funcMap).Parse(string(b)))

	err = templateAssets(headerFooter, rwt.mainTemplate)

	b, err = Asset("assets/list.html")
	if err != nil {
		return nil, err
	}
	rwt.listTemplate = template.Must(template.New("list").Parse(string(b)))

	err = templateAssets(headerFooter, rwt.listTemplate)

	b, err = Asset("assets/prism.js")
	if err != nil {
		return nil, err
	}
	rwt.prismTemplate = strings.Split(string(b), "LANGUAGES")

	return rwt, err
}

func templateAssets(s []string, t *template.Template) error {
	for _, asset := range s {
		b, err := Asset(asset)
		if err != nil {
			return err
		}

		t = template.Must(t.Parse(string(b)))
	}
	return nil
}

func (rwt *RWTxt) Serve() (err error) {
	go func() {
		lastDumped := time.Now().UTC()
		for {
			time.Sleep(120 * time.Second)
			lastModified, errGet := rwt.fs.LastModified()
			if errGet != nil {
				panic(errGet)
			}
			if time.Since(lastModified).Seconds() > 3 && time.Since(lastDumped).Seconds() > 10 {
				log.Debug("dumping")
				errDelete := rwt.fs.DeleteOldKeys()
				if errDelete != nil {
					log.Error(errDelete)
				}
				errDump := rwt.fs.DumpSQL()
				if errDump != nil {
					log.Error(errDump)
				}
				lastDumped = time.Now().UTC()
			}
		}
	}()
	log.Infof("listening on %v", rwt.Config.Bind)
	http.HandleFunc("/", rwt.Handler)
	return http.ListenAndServe(rwt.Config.Bind, nil)
}

func (rwt *RWTxt) isSignedIn(w http.ResponseWriter, r *http.Request, domain string) (signedin bool, domainkey string, defaultDomain string, domainList []string, domainKeys map[string]string) {
	domainKeys, defaultDomain = rwt.getDomainListCookie(w, r)
	domainList = make([]string, len(domainKeys))
	i := 0
	for domainName := range domainKeys {
		domainList[i] = domainName
		i++
		if domain == domainName {
			signedin = true
			domainkey = domainKeys[domainName]
		}
	}
	sort.Strings(domainList)
	return
}

func (rwt *RWTxt) getDomainListCookie(w http.ResponseWriter, r *http.Request) (domainKeys map[string]string, defaultDomain string) {
	startTime := time.Now().UTC()
	domainKeys = make(map[string]string)
	cookie, cookieErr := r.Cookie("rwtxt-domains")
	keysToUpdate := []string{}
	if cookieErr == nil {
		log.Debugf("got cookie: %s", cookie.Value)
		for _, key := range strings.Split(cookie.Value, ",") {
			startTime2 := time.Now().UTC()
			_, domainName, domainErr := rwt.fs.CheckKey(key)
			log.Debugf("checked key: %s [%s]", key, time.Since(startTime2))
			if domainErr == nil && domainName != "" {
				if defaultDomain == "" {
					defaultDomain = domainName
				}
				domainKeys[domainName] = key
				keysToUpdate = append(keysToUpdate, key)
			}
		}
	}
	domainKeys["public"] = ""
	if defaultDomain == "" {
		defaultDomain = "public"
	}
	log.Debugf("logged in domains: %+v [%s]", domainKeys, time.Since(startTime))
	go func() {
		if err := rwt.fs.UpdateKeys(keysToUpdate); err != nil {
			log.Debug(err)
		}
	}()
	return
}

func (rwt *RWTxt) Handler(w http.ResponseWriter, r *http.Request) {
	t := time.Now().UTC()
	err := rwt.Handle(w, r)
	if err != nil {
		log.Error(err)
	}
	log.Infof("%v %v %v %s", r.RemoteAddr, r.Method, r.URL.Path, time.Since(t))
}

func (rwt *RWTxt) Handle(w http.ResponseWriter, r *http.Request) (err error) {

	// very special paths
	if r.URL.Path == "/robots.txt" {
		// special path
		w.Write([]byte(`User-agent: * 
Disallow: /`))
	} else if r.URL.Path == "/favicon.ico" {
		// TODO
	} else if r.URL.Path == "/sitemap.xml" {
		// TODO
	} else if strings.HasPrefix(r.URL.Path, "/prism.js") {
		return rwt.handlePrism(w, r)
	} else if strings.HasPrefix(r.URL.Path, "/static") {
		// special path /static
		return rwt.handleStatic(w, r)
	}

	fields := strings.Split(r.URL.Path, "/")

	tr := NewTemplateRender(rwt)
	tr.Domain = "public"
	if len(fields) > 2 {
		tr.Page = strings.TrimSpace(strings.ToLower(fields[2]))
	}
	if len(fields) > 1 {
		tr.Domain = strings.TrimSpace(strings.ToLower(fields[1]))
	}

	tr.SignedIn, tr.DomainKey, tr.DefaultDomain, tr.DomainList, tr.DomainKeys = rwt.isSignedIn(w, r, tr.Domain)

	// get browser local time
	tr.getUTCOffsetFromCookie(r)

	if r.URL.Path == "/" {
		// special path /
		http.Redirect(w, r, "/"+tr.DefaultDomain, 302)
	} else if r.URL.Path == "/login" {
		// special path /login
		return tr.handleLogin(w, r)
	} else if r.URL.Path == "/ws" {
		// special path /ws
		return tr.handleWebsocket(w, r)
	} else if r.URL.Path == "/update" {
		// special path /login
		return tr.handleLoginUpdate(w, r)
	} else if r.URL.Path == "/logout" {
		// special path /logout
		return tr.handleLogout(w, r)
	} else if r.URL.Path == "/upload" {
		// special path /upload
		return tr.handleUpload(w, r)
	} else if tr.Page == "new" {
		// special path /upload
		http.Redirect(w, r, "/"+tr.DefaultDomain+"/"+rwt.createPage(tr.DefaultDomain).ID, 302)
		return
	} else if strings.HasPrefix(r.URL.Path, "/uploads") {
		// special path /uploads
		return tr.handleUploads(w, r, tr.Page)
	} else if tr.Domain != "" && tr.Page == "" {
		if r.URL.Query().Get("q") != "" {
			if tr.Domain == "public" && !rwt.Config.Private {
				err = fmt.Errorf("cannot search public")
				http.Redirect(w, r, "/"+tr.Domain+"?m="+base64.URLEncoding.EncodeToString([]byte(err.Error())), 302)
				return
			}
			return tr.handleSearch(w, r, tr.Domain, r.URL.Query().Get("q"))
		}
		// domain exists, handle normally
		return tr.handleMain(w, r)
	} else if tr.Domain != "" && tr.Page != "" {
		log.Debugf("[%s/%s]", tr.Domain, tr.Page)
		if tr.Page == "list" {
			if tr.Domain == "public" && !rwt.Config.Private {
				err = fmt.Errorf("cannot list public")
				http.Redirect(w, r, "/"+tr.Domain+"?m="+base64.URLEncoding.EncodeToString([]byte(err.Error())), 302)
				return
			}

			files, _ := rwt.fs.GetAll(tr.Domain, tr.RWTxtConfig.OrderByCreated)
			for i := range files {
				files[i].Data = ""
				files[i].DataHTML = template.HTML("")
			}
			return tr.handleList(w, r, "All", files)
		} else if tr.Page == "export" {
			return tr.handleExport(w, r)
		}
		return tr.handleViewEdit(w, r)
	}
	return
}

func (rwt *RWTxt) handlePrism(w http.ResponseWriter, r *http.Request) (err error) {
	prismJS := rwt.prismTemplate[0]
	languageString, ok := r.URL.Query()["l"]
	if ok && len(languageString) > 0 {
		for _, lang := range strings.Split(languageString[0], ",") {
			if _, ok2 := languageCSS[lang]; ok2 {
				prismJS += languageCSS[lang]
			}
		}
	}
	prismJS += rwt.prismTemplate[1]
	w.Header().Set("Cache-Control", "public, max-age=7776000")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/javascript")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, err = gz.Write([]byte(prismJS))
	return
}

func (rwt *RWTxt) handleStatic(w http.ResponseWriter, r *http.Request) (err error) {
	page := r.URL.Path
	e := `"` + r.URL.Path + `"`

	//https://www.sanarias.com/blog/115LearningHTTPcachinginGo
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Etag", e)
	w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
	if match := r.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, e) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	w.Header().Set("Content-Encoding", "gzip")
	if strings.HasPrefix(page, "/static") {
		page = "assets/" + strings.TrimPrefix(page, "/static/")
		b, _ := Asset(page + ".gz")
		if strings.Contains(page, ".js") {
			w.Header().Set("Content-Type", "text/javascript")
		} else if strings.Contains(page, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.Contains(page, ".png") {
			w.Header().Set("Content-Type", "image/png")
		} else if strings.Contains(page, ".json") {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write(b)
	}

	return
}

// createPage throws error if domain does not exist
func (rwt *RWTxt) createPage(domain string) (f db.File) {
	f = db.File{
		ID:       utils.UUID(),
		Created:  time.Now().UTC(),
		Domain:   domain,
		Modified: time.Now().UTC(),
	}
	err := rwt.fs.Save(f)
	if err != nil {
		log.Debug(err)
	}
	return
}

func (rwt *RWTxt) addSimilar(domain string, fileid string) (err error) {
	files, err := rwt.fs.GetAll(domain)
	documents := []string{}
	ids := []string{}
	maindocument := ""
	for _, file := range files {
		if file.ID == fileid {
			maindocument = file.Data
			continue
		}
		ids = append(ids, file.ID)
		documents = append(documents, file.Data)
	}

	ds, err := documentsimilarity.New(documents)
	if err != nil {
		return
	}

	similarities, err := ds.JaccardSimilarity(maindocument)
	if err != nil {
		return
	}

	if len(similarities) > 5 {
		similarities = similarities[:5]
	}
	similarIds := make([]string, len(similarities))
	for i, similarity := range similarities {
		similarIds[i] = ids[similarity.Index]
	}

	err = rwt.fs.SetSimilar(fileid, similarIds)
	return
}

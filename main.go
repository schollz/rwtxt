package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/websocket"
	"github.com/schollz/documentsimilarity"
	"github.com/schollz/rwtxt/src/db"
	"github.com/schollz/rwtxt/src/utils"
)

const (
	introText = "This note is empty. Click to edit it."
)

var viewEditTemplate *template.Template
var mainTemplate *template.Template
var loginTemplate *template.Template
var listTemplate *template.Template
var fs *db.FileSystem

type TemplateRender struct {
	Title             string
	Page              string
	Rendered          template.HTML
	File              db.File
	IntroText         template.JS
	Rows              int
	RandomUUID        string
	Domain            string
	DomainID          int
	DomainKey         string
	DomainIsPrivate   bool
	DomainValue       template.HTMLAttr
	DomainList        []string
	DomainKeys        map[string]string
	DefaultDomain     string
	SignedIn          bool
	Message           string
	NumResults        int
	Files             []db.File
	MostActiveList    []db.File
	SimilarFiles      []db.File
	Search            string
	DomainExists      bool
	ShowCookieMessage bool
	EditOnly          bool
}

func init() {
	var err error

	b, err := Asset("assets/viewedit.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(template.New("viewedit").Parse(string(b)))
	b, err = Asset("assets/header.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))
	b, err = Asset("assets/footer.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))

	b, err = Asset("assets/main.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(template.New("main").Parse(string(b)))
	b, err = Asset("assets/header.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))
	b, err = Asset("assets/footer.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))

	b, err = Asset("assets/list.html")
	if err != nil {
		panic(err)
	}
	listTemplate = template.Must(template.New("main").Parse(string(b)))
	b, err = Asset("assets/header.html")
	if err != nil {
		panic(err)
	}
	listTemplate = template.Must(listTemplate.Parse(string(b)))
	b, err = Asset("assets/footer.html")
	if err != nil {
		panic(err)
	}
	listTemplate = template.Must(listTemplate.Parse(string(b)))
}

var dbName string
var Version string

func main() {
	var err error
	var debug = flag.Bool("debug", false, "debug mode")
	var showVersion = flag.Bool("v", false, "show version")
	var database = flag.String("db", "rwtxt.db", "name of the database")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}
	if *debug {
		err = setLogLevel("debug")
		db.SetLogLevel("debug")
	} else {
		err = setLogLevel("info")
		db.SetLogLevel("info")
	}
	if err != nil {
		panic(err)
	}
	dbName = *database
	defer log.Flush()

	err = serve()
	if err != nil {
		log.Error(err)
	}
}

type Payload struct {
	ID        string `json:"id,omitempty"`
	DomainKey string `json:"domain_key,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Data      string `json:"data,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Message   string `json:"message,omitempty"`
	Success   bool   `json:"success"`
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serve() (err error) {
	fs, err = db.New(dbName)
	if err != nil {
		log.Error(err)
		return
	}

	go func() {
		lastDumped := time.Now()
		for {
			time.Sleep(120 * time.Second)
			lastModified, errGet := fs.LastModified()
			if errGet != nil {
				panic(errGet)
			}
			if time.Since(lastModified).Seconds() > 3 && time.Since(lastDumped).Seconds() > 10 {
				log.Debug("dumping")
				errDelete := fs.DeleteOldKeys()
				if errDelete != nil {
					log.Error(errDelete)
				}
				errDump := fs.DumpSQL()
				if errDump != nil {
					log.Error(errDump)
				}
				lastDumped = time.Now()
			}
		}
	}()
	log.Info("running on port 8152")
	http.HandleFunc("/", handler)
	return http.ListenAndServe(":8152", nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	err := handle(w, r)
	if err != nil {
		log.Error(err)
	}
	log.Infof("%v %v %v %s", r.RemoteAddr, r.Method, r.URL.Path, time.Since(t))
}

func (tr *TemplateRender) handleSearch(w http.ResponseWriter, r *http.Request, domain, query string) (err error) {
	_, ispublic, _ := fs.GetDomainFromName(domain)
	if !tr.SignedIn && !ispublic {
		return tr.handleMain(w, r, "need to log in to search")
	}
	files, errGet := fs.Find(query, tr.Domain)
	if errGet != nil {
		return errGet
	}
	return tr.handleList(w, r, query, files)
}

func (tr *TemplateRender) handleList(w http.ResponseWriter, r *http.Request, query string, files []db.File) (err error) {
	// show the list page
	tr.Title = query + " pages"
	tr.Files = files
	tr.NumResults = len(files)
	tr.Search = query
	tr.RandomUUID = utils.UUID()

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return listTemplate.Execute(gz, tr)
}

func isSignedIn(w http.ResponseWriter, r *http.Request, domain string) (signedin bool, domainkey string, defaultDomain string, domainList []string, domainKeys map[string]string) {
	domainKeys, defaultDomain = getDomainListCookie(w, r)
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

func (tr TemplateRender) updateDomainCookie(w http.ResponseWriter, r *http.Request) (cookie http.Cookie) {
	delete(tr.DomainKeys, "public")
	tr.DomainKeys[tr.Domain] = tr.DomainKey
	log.Debugf("updated domain keys: %+v", tr.DomainKeys)

	// add the current one as default
	domainKeyList := []string{tr.DomainKey}

	// add the others
	for domainName := range tr.DomainKeys {
		if domainName != tr.Domain {
			domainKeyList = append(domainKeyList, tr.DomainKeys[domainName])
		}
	}

	log.Debugf("setting new list: %+v", domainKeyList)
	// return the new cookie
	return http.Cookie{
		Name:    "rwtxt-domains",
		Value:   strings.Join(domainKeyList, ","),
		Expires: time.Now().Add(365 * 24 * time.Hour),
	}
}

func getDomainListCookie(w http.ResponseWriter, r *http.Request) (domainKeys map[string]string, defaultDomain string) {
	startTime := time.Now()
	domainKeys = make(map[string]string)
	cookie, cookieErr := r.Cookie("rwtxt-domains")
	keysToUpdate := []string{}
	if cookieErr == nil {
		log.Debugf("got cookie: %s", cookie.Value)
		for _, key := range strings.Split(cookie.Value, ",") {
			startTime2 := time.Now()
			domainName, domainErr := fs.CheckKey(key)
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
		if err := fs.UpdateKeys(keysToUpdate); err != nil {
			log.Debug(err)
		}
	}()
	return
}

func (tr *TemplateRender) handleMain(w http.ResponseWriter, r *http.Request, message string) (err error) {

	// set the default domain if it doesn't exist
	if tr.SignedIn && tr.DefaultDomain != tr.Domain {
		cookie := tr.updateDomainCookie(w, r)
		http.SetCookie(w, &cookie)
	}

	// create a page to write to
	newFile := db.File{
		ID:       utils.UUID(),
		Created:  time.Now(),
		Domain:   tr.Domain,
		Modified: time.Now(),
	}
	defer func() {
		go func() {
			// premediate the page
			err := fs.Save(newFile)
			if err != nil {
				log.Debug(err)
			}
		}()
	}()
	tr.RandomUUID = newFile.ID

	// delete this
	_, ispublic, domainErr := fs.GetDomainFromName(tr.Domain)
	signedin := tr.SignedIn
	if domainErr != nil {
		// domain does NOT exist
		signedin = false
	}
	tr.SignedIn = signedin
	tr.DomainIsPrivate = !ispublic && tr.Domain != "public"
	tr.DomainExists = domainErr == nil
	tr.Files, err = fs.GetTopX(tr.Domain, 10)
	if err != nil {
		log.Debug(err)
	}

	tr.MostActiveList, _ = fs.GetTopXMostViews(tr.Domain, 10)
	tr.Title = "rwtxt"
	tr.Message = message
	tr.DomainValue = template.HTMLAttr(`value="` + tr.Domain + `"`)

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return mainTemplate.Execute(gz, tr)
}

func (tr *TemplateRender) handleLogout(w http.ResponseWriter, r *http.Request) (err error) {
	tr.Domain = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("d")))

	// delete all cookies
	_, err = r.Cookie("rwtxt-domains")
	if err == nil {
		c := &http.Cookie{
			Name:     "rwtxt-domains",
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
		}
		http.SetCookie(w, c)
	}

	return tr.handleMain(w, r, "You are not logged in.")
}

func (tr *TemplateRender) handleLogin(w http.ResponseWriter, r *http.Request) (err error) {
	tr.Domain = strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	password := strings.TrimSpace(r.FormValue("password"))
	if tr.Domain == "public" || tr.Domain == "" {
		tr.Domain = "public"
		return tr.handleMain(w, r, "")
	}
	if password == "" {
		tr.Domain = "public"
		return tr.handleMain(w, r, "domain key cannot be empty")
	}
	var key string

	// check if exists
	_, _, err = fs.GetDomainFromName(tr.Domain)
	if err != nil {
		// domain doesn't exist, create it
		log.Debugf("domain '%s' doesn't exist, creating it", tr.Domain)
		err = fs.SetDomain(tr.Domain, password)
		if err != nil {
			log.Error(err)
			tr.Domain = "public"
			return tr.handleMain(w, r, err.Error())
		}
	}
	tr.DomainKey, err = fs.SetKey(tr.Domain, password)
	if err != nil {
		tr.Domain = "public"
		return tr.handleMain(w, r, err.Error())
	}

	log.Debugf("new key: %s", key)
	// set domain password
	cookie := tr.updateDomainCookie(w, r)
	http.SetCookie(w, &cookie)
	http.Redirect(w, r, "/"+tr.Domain, 302)
	return nil
}

func (tr *TemplateRender) handleLoginUpdate(w http.ResponseWriter, r *http.Request) (err error) {
	tr.DomainKey = strings.TrimSpace(strings.ToLower(r.FormValue("domain_key")))
	tr.Domain = strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	password := strings.TrimSpace(r.FormValue("password"))
	isPublic := strings.TrimSpace(r.FormValue("ispublic")) == "on"
	if tr.Domain == "public" || tr.Domain == "" {
		tr.Domain = "public"
		return tr.handleMain(w, r, "cannot modify public")
	}

	// check that the key is valid
	domainFound, err := fs.CheckKey(tr.DomainKey)
	if err != nil || tr.Domain != domainFound {
		if err != nil {
			log.Debug(err)
		}
		return tr.handleMain(w, r, err.Error())
	}

	err = fs.UpdateDomain(tr.Domain, password, isPublic)
	message := "settings updated"
	if password != "" {
		message = "password updated"
	}
	if err != nil {
		message = err.Error()
	}
	return tr.handleMain(w, r, message)
}

func (tr *TemplateRender) handleWebsocket(w http.ResponseWriter, r *http.Request) (err error) {
	// handle websockets on this page
	c, errUpgrade := wsupgrader.Upgrade(w, r, nil)
	if errUpgrade != nil {
		return errUpgrade
	}
	defer c.Close()
	domainChecked := false
	domainValidated := false
	var editFile db.File
	var p Payload
	for {
		err := c.ReadJSON(&p)
		if err != nil {
			log.Debug("read:", err)
			if editFile.ID != "" {
				log.Debugf("saving editing of /%s/%s", editFile.Domain, editFile.ID)
				if editFile.Domain != "public" {
					err = addSimilar(editFile.Domain, editFile.ID)
					if err != nil {
						log.Error(err)
					}
				}
			}
			break
		}
		// log.Debugf("recv: %v", p)

		if !domainChecked {
			domainChecked = true
			if p.Domain == "public" {
				domainValidated = true
			} else {
				_, keyErr := fs.CheckKey(p.DomainKey)
				if keyErr == nil {
					domainValidated = true
				}
			}
		}

		// save it
		if p.ID != "" && domainValidated {
			if p.Domain == "" {
				p.Domain = "public"
			}
			data := strings.TrimSpace(p.Data)
			if data == introText {
				data = ""
			}
			editFile = db.File{
				ID:      p.ID,
				Slug:    p.Slug,
				Data:    data,
				Created: time.Now(),
				Domain:  p.Domain,
			}
			err = fs.Save(editFile)
			if err != nil {
				log.Error(err)
			}
			fs, _ := fs.Get(p.Slug, p.Domain)

			err = c.WriteJSON(Payload{
				ID:      p.ID,
				Slug:    p.Slug,
				Message: "unique_slug",
				Success: len(fs) < 2,
			})
			if err != nil {
				log.Debug("write:", err)
				break
			}
		} else {
			log.Debug("not saving")
			err = c.WriteJSON(Payload{
				Message: "not saving",
			})
			if err != nil {
				log.Debug("write:", err)
				break
			}
		}
	}
	return
}

func handleStatic(w http.ResponseWriter, r *http.Request) (err error) {
	page := r.URL.Path
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Cache-Control", "public, max-age=7776000")
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

func (tr *TemplateRender) handleViewEdit(w http.ResponseWriter, r *http.Request) (err error) {
	// handle new page
	// get edit url parameter
	log.Debugf("loading %s", tr.Page)
	havePage, err := fs.Exists(tr.Page, tr.Domain)
	if err != nil {
		return
	}
	initialMarkdown := ""
	var f db.File

	// check if domain is public and exists
	_, ispublic, errGet := fs.GetDomainFromName(tr.Domain)
	if errGet == nil && !tr.SignedIn && !ispublic {
		return tr.handleMain(w, r, "domain is not public, sign in first")
	}

	if havePage {
		var files []db.File
		files, err = fs.Get(tr.Page, tr.Domain)
		if err != nil {
			log.Error(err)
			return tr.handleMain(w, r, err.Error())
		}
		if len(files) > 1 {
			return tr.handleList(w, r, tr.Page, files)
		} else {
			f = files[0]
		}
		tr.SimilarFiles, err = fs.GetSimilar(f.ID)
		if err != nil {
			log.Error(err)
		}
	} else {
		uuid := utils.UUID()
		f = db.File{
			ID:       uuid,
			Created:  time.Now(),
			Domain:   tr.Domain,
			Modified: time.Now(),
		}
		f.Slug = tr.Page
		f.Data = ""
		err = fs.Save(f)
		if err != nil {
			return tr.handleMain(w, r, "domain does not exist")
		}
		log.Debugf("saved: %+v", f)
		http.Redirect(w, r, "/"+tr.Domain+"/"+tr.Page, 302)
		return
	}
	initialMarkdown += "\n\n" + f.Data
	// if f.Data == "" {
	// 	f.Data = introText
	// }
	// update the view count
	go func() {
		err := fs.UpdateViews(f)
		if err != nil {
			log.Error(err)
		}
	}()

	tr.Title = f.Slug
	tr.Rendered = utils.RenderMarkdownToHTML(initialMarkdown)
	tr.File = f
	tr.IntroText = template.JS(introText)
	tr.Rows = len(strings.Split(string(utils.RenderMarkdownToHTML(initialMarkdown)), "\n")) + 1
	tr.EditOnly = strings.TrimSpace(f.Data) == ""

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	log.Debug(strings.TrimSpace(f.Data))

	return viewEditTemplate.Execute(gz, tr)

}

func (tr *TemplateRender) handleUploads(w http.ResponseWriter, r *http.Request, id string) (err error) {
	log.Debug("getting ", id)
	name, data, _, err := fs.GetBlob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Cache-Control", "public, max-age=7776000")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+name+`"`,
	)
	w.Write(data)
	return
}

func (tr *TemplateRender) handleUpload(w http.ResponseWriter, r *http.Request) (err error) {
	domain := r.URL.Query().Get("domain")
	if !tr.SignedIn || domain == "public" {
		http.Error(w, "need to be logged in", http.StatusForbidden)
		return
	}

	file, info, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := fmt.Sprintf("sha256-%x", h.Sum(nil))

	// copy file to buffer
	file.Seek(0, io.SeekStart)
	var fileData bytes.Buffer
	gzipWriter := gzip.NewWriter(&fileData)
	_, err = io.Copy(gzipWriter, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gzipWriter.Close()

	// save file
	err = fs.SaveBlob(id, info.Filename, fileData.Bytes())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", "/uploads/"+id+"?filename="+url.QueryEscape(info.Filename))
	_, err = w.Write([]byte("ok"))
	return
}

func handle(w http.ResponseWriter, r *http.Request) (err error) {
	// very special paths
	if r.URL.Path == "/robots.txt" {
		// special path
		w.Write([]byte(`User-agent: * 
Disallow: /`))
	} else if r.URL.Path == "/favicon.ico" {
		// TODO
	} else if r.URL.Path == "/sitemap.xml" {
		// TODO
	} else if strings.HasPrefix(r.URL.Path, "/static") {
		// special path /static
		return handleStatic(w, r)
	}

	fields := strings.Split(r.URL.Path, "/")

	tr := new(TemplateRender)
	tr.Domain = "public"
	if len(fields) > 2 {
		tr.Page = strings.TrimSpace(strings.ToLower(fields[2]))
	}
	if len(fields) > 1 {
		tr.Domain = strings.TrimSpace(strings.ToLower(fields[1]))
	}

	tr.SignedIn, tr.DomainKey, tr.DefaultDomain, tr.DomainList, tr.DomainKeys = isSignedIn(w, r, tr.Domain)

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
		http.Redirect(w, r, "/"+tr.DefaultDomain+"/"+createPage(tr.DefaultDomain).ID, 302)
		return
	} else if strings.HasPrefix(r.URL.Path, "/uploads") {
		// special path /uploads
		return tr.handleUploads(w, r, tr.Page)
	} else if tr.Domain != "" && tr.Page == "" {
		if tr.Domain == "public" {
			return tr.handleMain(w, r, "can't search public")
		}
		if r.URL.Query().Get("q") != "" {
			return tr.handleSearch(w, r, tr.Domain, r.URL.Query().Get("q"))
		}
		// domain exists, handle normally
		return tr.handleMain(w, r, "")
	} else if tr.Domain != "" && tr.Page != "" {
		if tr.Page == "list" {
			if tr.Domain == "public" {
				return tr.handleMain(w, r, "can't list public")
			}

			files, _ := fs.GetAll(tr.Domain)
			for i := range files {
				files[i].Data = ""
				files[i].DataHTML = template.HTML("")
			}
			return tr.handleList(w, r, "All", files)
		}
		return tr.handleViewEdit(w, r)
	}
	return
}

// setLogLevel determines the log level
func setLogLevel(level string) (err error) {

	// https://en.wikipedia.org/wiki/ANSI_escape_code#3/4_bit
	// https://github.com/cihub/seelog/wiki/Log-levels
	appConfig := `
	<seelog minlevel="` + level + `">
	<outputs formatid="stdout">
	<filter levels="debug,trace">
		<console formatid="debug"/>
	</filter>
	<filter levels="info">
		<console formatid="info"/>
	</filter>
	<filter levels="critical,error">
		<console formatid="error"/>
	</filter>
	<filter levels="warn">
		<console formatid="warn"/>
	</filter>
	</outputs>
	<formats>
		<format id="stdout"   format="%Date %Time [%LEVEL] %File %FuncShort:%Line %Msg %n" />
		<format id="debug"   format="%Date %Time %EscM(37)[%LEVEL]%EscM(0) %File %FuncShort:%Line %Msg %n" />
		<format id="info"    format="%Date %Time %EscM(36)[%LEVEL]%EscM(0) %File %FuncShort:%Line %Msg %n" />
		<format id="warn"    format="%Date %Time %EscM(33)[%LEVEL]%EscM(0) %File %FuncShort:%Line %Msg %n" />
		<format id="error"   format="%Date %Time %EscM(31)[%LEVEL]%EscM(0) %File %FuncShort:%Line %Msg %n" />
	</formats>
	</seelog>
	`
	logger, err := log.LoggerFromConfigAsBytes([]byte(appConfig))
	if err != nil {
		return
	}
	log.ReplaceLogger(logger)
	return
}

// createPage throws error if domain does not exist
func createPage(domain string) (f db.File) {
	f = db.File{
		ID:       utils.UUID(),
		Created:  time.Now(),
		Domain:   domain,
		Modified: time.Now(),
	}
	err := fs.Save(f)
	if err != nil {
		log.Debug(err)
	}
	return
}

func addSimilar(domain string, fileid string) (err error) {
	files, err := fs.GetAll(domain)
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

	err = fs.SetSimilar(fileid, similarIds)
	return
}

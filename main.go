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
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/websocket"
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
	SignedIn          bool
	Message           string
	NumResults        int
	Files             []db.File
	Search            string
	DomainExists      bool
	ShowCookieMessage bool
}

func init() {
	var err error

	b, err := Asset("viewedit.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(template.New("viewedit").Parse(string(b)))
	b, err = Asset("header.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))
	b, err = Asset("footer.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))

	b, err = Asset("main.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(template.New("main").Parse(string(b)))
	b, err = Asset("header.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))
	b, err = Asset("footer.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))

	b, err = Asset("list.html")
	if err != nil {
		panic(err)
	}
	listTemplate = template.Must(template.New("main").Parse(string(b)))
	b, err = Asset("header.html")
	if err != nil {
		panic(err)
	}
	listTemplate = template.Must(listTemplate.Parse(string(b)))
	b, err = Asset("footer.html")
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

func handleSearch(w http.ResponseWriter, r *http.Request, domain, query string) (err error) {
	_, ispublic, _ := fs.GetDomainFromName(domain)
	signedin, _, _ := isSignedIn(w, r, domain)
	if !signedin && !ispublic {
		return handleMain(w, r, domain, "need to log in to search")
	}
	files, errGet := fs.Find(query, domain)
	if errGet != nil {
		return errGet
	}
	return handleList(w, r, domain, query, files)
}

func handleList(w http.ResponseWriter, r *http.Request, domain string, query string, files []db.File) (err error) {
	// show the list page
	signedin, _, _ := isSignedIn(w, r, domain)
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return listTemplate.Execute(gz, TemplateRender{
		Title:      query + " pages",
		Domain:     domain,
		Files:      files,
		NumResults: len(files),
		Search:     query,
		RandomUUID: utils.UUID(),
		SignedIn:   signedin,
	})
}

func isSignedIn(w http.ResponseWriter, r *http.Request, domain string) (signedin bool, domainkey string, defaultDomain string) {
	// check for default domain
	defaultDomainCookie, cookieErr := r.Cookie("rwtxt-default-domain")
	if cookieErr == nil {
		// domain exists, handle normally
		defaultDomain = defaultDomainCookie.Value
	}
	if domain == "" {
		domain = "public"
	}
	if domain == "public" {
		return
	}

	// get domain key
	cookie, cookieErr := r.Cookie(domain)
	if cookieErr == nil {
		log.Debugf("got cookie %+v", cookie.Value)
		domainkey = cookie.Value
		signedin = true
	}
	return
}

func handleMain(w http.ResponseWriter, r *http.Request, domain string, message string) (err error) {
	// check if first time user
	signedin, domainKey, defaultDomain := isSignedIn(w, r, domain)

	// set the default domain if it doesn't exist
	var showCookieMessage bool
	if defaultDomain == "" {
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "rwtxt-default-domain", Value: "public", Expires: expiration}
		http.SetCookie(w, &cookie)
		showCookieMessage = true
	}

	_, ispublic, domainErr := fs.GetDomainFromName(domain)
	files, err := fs.GetTopX(domain, 10)
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return mainTemplate.Execute(gz, TemplateRender{
		Title:             "rwtxt",
		Message:           message,
		Domain:            domain,
		RandomUUID:        utils.UUID(),
		SignedIn:          signedin,
		Files:             files,
		DomainExists:      domainErr == nil,
		DomainIsPrivate:   !ispublic && domain != "public",
		DomainKey:         domainKey,
		ShowCookieMessage: showCookieMessage,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) (err error) {
	domain := r.URL.Query().Get("d")

	// delete default domain cookie
	_, err = r.Cookie("rwtxt-default-domain")
	if err == nil {
		c := &http.Cookie{
			Name:     "rwtxt-default-domain",
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
		}
		http.SetCookie(w, c)
	}

	// delete domain password cookie
	cookie, err := r.Cookie(domain)
	if err == nil {
		err = fs.DeleteKey(cookie.Value)
		if err != nil {
			log.Error(err)
		}
		c := &http.Cookie{
			Name:     domain,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
		}
		http.SetCookie(w, c)
		http.Redirect(w, r, "/", 302)
		return
	}
	return handleMain(w, r, domain, "You are not logged in.")
}

func handleLogin(w http.ResponseWriter, r *http.Request) (err error) {
	domain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	password := strings.TrimSpace(r.FormValue("password"))
	if domain == "public" || domain == "" {
		return handleMain(w, r, "public", "")
	}
	if password == "" {
		return handleMain(w, r, "public", "domain key cannot be empty")
	}
	var key string

	// check if exists
	_, _, err = fs.GetDomainFromName(domain)
	if err != nil {
		// domain doesn't exist, create it
		log.Debugf("domain '%s' doesn't exist, creating it", domain)
		err = fs.SetDomain(domain, password)
		if err != nil {
			log.Error(err)
			return handleMain(w, r, "public", err.Error())
		}
	}
	key, err = fs.SetKey(domain, password)
	if err != nil {
		return handleMain(w, r, "public", err.Error())
	}

	log.Debugf("new key: %s", key)
	// set domain password
	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: domain, Value: key, Expires: expiration}
	http.SetCookie(w, &cookie)
	// set domain default
	cookie2 := http.Cookie{Name: "rwtxt-default-domain", Value: domain, Expires: expiration}
	http.SetCookie(w, &cookie2)
	http.Redirect(w, r, "/"+domain, 302)
	return nil
}

func handleLoginUpdate(w http.ResponseWriter, r *http.Request) (err error) {
	domainKey := strings.TrimSpace(strings.ToLower(r.FormValue("domain_key")))
	domain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	password := strings.TrimSpace(r.FormValue("password"))
	isPublic := strings.TrimSpace(r.FormValue("ispublic")) == "on"
	if domain == "public" || domain == "" {
		return handleMain(w, r, "public", "cannot modify public")
	}

	// check that the key is valid
	domainFound, err := fs.CheckKey(domainKey)
	if err != nil || domain != domainFound {
		if err != nil {
			log.Debug(err)
		}
		return handleMain(w, r, domain, err.Error())
	}

	err = fs.UpdateDomain(domain, password, isPublic)
	message := "settings updated"
	if password != "" {
		message = "password updated"
	}
	if err != nil {
		message = err.Error()
	}
	return handleMain(w, r, domain, message)
}

func handleWebsocket(w http.ResponseWriter, r *http.Request) (err error) {
	// handle websockets on this page
	c, errUpgrade := wsupgrader.Upgrade(w, r, nil)
	if errUpgrade != nil {
		return errUpgrade
	}
	defer c.Close()
	domainChecked := false
	domainValidated := false
	var p Payload
	for {
		err := c.ReadJSON(&p)
		if err != nil {
			log.Debug("read:", err)
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
			log.Debug("saving")
			if p.Domain == "" {
				p.Domain = "public"
			}
			data := strings.TrimSpace(p.Data)
			if data == introText {
				data = ""
			}

			err = fs.Save(db.File{
				ID:      p.ID,
				Slug:    p.Slug,
				Data:    data,
				Created: time.Now(),
				Domain:  p.Domain,
			})
			log.Debug("saved")
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
	log.Debug(page)
	if strings.HasSuffix(page, "rwtxt.js") {
		b, _ := Asset("rwtxt.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(b)
	} else if strings.HasSuffix(page, "normalize.css") {
		b, _ := Asset("normalize.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "dropzone.css") {
		b, _ := Asset("dropzone.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "rwtxt.css") {
		b, _ := Asset("rwtxt.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "prism.css") {
		b, _ := Asset("prism.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "dropzone.js") {
		b, _ := Asset("dropzone.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(b)
	} else if strings.HasSuffix(page, "prism.js") {
		b, _ := Asset("prism.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(b)
	} else if strings.HasSuffix(page, "logo.png") {
		b, _ := Asset("logo.png")
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	}
	return
}

func handleViewEdit(w http.ResponseWriter, r *http.Request, domain, page string) (err error) {
	// handle new page
	// get edit url parameter
	log.Debugf("loading %s", page)
	havePage, err := fs.Exists(page, domain)
	if err != nil {
		return
	}
	initialMarkdown := ""
	var f db.File
	log.Debugf("%s %s %v", page, domain, havePage)

	// if domainexists, and is not signed in and is not public,
	// then restrict access
	signedIn, domainKey, _ := isSignedIn(w, r, domain)
	// check if domain is public and exists
	_, ispublic, errGet := fs.GetDomainFromName(domain)
	if errGet == nil && !signedIn && !ispublic {
		return handleMain(w, r, domain, "domain is not public, sign in first")
	}

	if havePage {
		var files []db.File
		files, err = fs.Get(page, domain)
		if err != nil {
			log.Error(err)
			return handleMain(w, r, domain, err.Error())
		}
		if len(files) > 1 {
			return handleList(w, r, domain, page, files)
		} else {
			f = files[0]
		}
	} else {
		uuid := utils.UUID()
		f = db.File{
			ID:       uuid,
			Created:  time.Now(),
			Domain:   domain,
			Modified: time.Now(),
		}
		f.Slug = page
		f.Data = ""
		err = fs.Save(f)
		if err != nil {
			log.Error(err)
			return handleMain(w, r, domain, "domain does not exist")
		}
		log.Debugf("saved: %+v", f)
		http.Redirect(w, r, "/"+domain+"/"+page+"?edit=1", 302)
		return
	}
	initialMarkdown += "\n\n" + f.Data
	if f.Data == "" {
		f.Data = introText
	}
	// update the view count
	go func() {
		err := fs.UpdateViews(f)
		if err != nil {
			log.Error(err)
		}
	}()

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return viewEditTemplate.Execute(gz, TemplateRender{
		Page:      page,
		Rendered:  utils.RenderMarkdownToHTML(initialMarkdown),
		File:      f,
		IntroText: template.JS(introText),
		Title:     f.Slug,
		Rows:      len(strings.Split(string(utils.RenderMarkdownToHTML(initialMarkdown)), "\n")) + 1,
		Domain:    domain,
		DomainKey: domainKey,
		SignedIn:  signedIn,
	})

}

func handleUploads(w http.ResponseWriter, r *http.Request, id string) (err error) {
	log.Debug("getting ", id)
	name, data, err := fs.GetBlob(id)
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

func handleUpload(w http.ResponseWriter, r *http.Request) (err error) {
	domain := r.URL.Query().Get("domain")
	signedIn, _, _ := isSignedIn(w, r, domain)
	if !signedIn || domain == "public" {
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
	fields := strings.Split(r.URL.Path, "/")
	domain := "public"
	page := ""

	if len(fields) > 2 {
		page = strings.TrimSpace(strings.ToLower(fields[2]))
	}
	if len(fields) > 1 {
		domain = strings.TrimSpace(strings.ToLower(fields[1]))
	}
	if r.URL.Path == "/" {
		// special path /

		// check to see if there is a default domain
		cookie, cookieErr := r.Cookie("rwtxt-default-domain")
		if cookieErr == nil {
			_, _, domainErr := fs.GetDomainFromName(domain)
			if domainErr == nil {
				// domain exists, redirect to it
				http.Redirect(w, r, "/"+cookie.Value, 302)
				return
			}
		}
		http.Redirect(w, r, "/public", 302)
	} else if r.URL.Path == "/robots.txt" {
		// special path
		w.Write([]byte(`User-agent: * 
Disallow: /`))
	} else if r.URL.Path == "/favicon.ico" {
		// favicon

	} else if r.URL.Path == "/sitemap.xml" {
		// favicon

	} else if r.URL.Path == "/ws" {
		// special path /ws
		return handleWebsocket(w, r)
	} else if strings.HasPrefix(r.URL.Path, "/static") {
		// special path /static
		return handleStatic(w, r)
	} else if r.URL.Path == "/login" {
		// special path /login
		return handleLogin(w, r)
	} else if r.URL.Path == "/update" {
		// special path /login
		return handleLoginUpdate(w, r)
	} else if r.URL.Path == "/logout" {
		// special path /logout
		return handleLogout(w, r)
	} else if r.URL.Path == "/upload" {
		// special path /upload
		return handleUpload(w, r)
	} else if strings.HasPrefix(r.URL.Path, "/uploads") {
		// special path /uploads
		return handleUploads(w, r, page)
	} else if domain != "" && page == "" {
		if r.URL.Query().Get("q") != "" {
			return handleSearch(w, r, domain, r.URL.Query().Get("q"))
		}
		// check to see if domain exists
		cookie, cookieErr := r.Cookie("rwtxt-default-domain")
		_, _, domainErr := fs.GetDomainFromName(domain)
		if domainErr != nil && cookieErr == nil {
			log.Debug(domainErr)
			// we are trying to goto a page that doesn't exist as a domain
			// automatically create a new page for editing in the default domain
			http.Redirect(w, r, "/"+cookie.Value+"/"+domain+"?edit=1", 302)
			return
		}

		// check to see if page exists in public domain and redirect to it
		fs, _ := fs.Get(domain, "public")
		if len(fs) > 0 {
			http.Redirect(w, r, "/public/"+domain, 302)
			return
		}

		// domain exists, handle normally
		return handleMain(w, r, domain, "")

		return
	} else if domain != "" && page != "" {
		log.Debug("handle view edit")
		return handleViewEdit(w, r, domain, page)
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

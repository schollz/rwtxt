package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/websocket"
	"github.com/schollz/rwtxt/src/db"
	"github.com/schollz/rwtxt/src/utils"
	swearjar "github.com/schollz/swearjar-go"
)

const (
	introText = "This note is empty. Click to edit it."
)

var viewEditTemplate *template.Template
var mainTemplate *template.Template
var loginTemplate *template.Template
var listTemplate *template.Template
var fs *db.FileSystem
var swearChecker swearjar.Swears

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

	swearChecker, err = swearjar.Load()
	if err != nil {
		panic(err)
	}

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

func main() {
	defer log.Flush()

	err := setLogLevel("debug")
	if err != nil {
		panic(err)
	}
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
	fs, err = db.New("rwtxt.db")
	if err != nil {
		log.Error(err)
		return
	}
	go func() {
		lastDumped := time.Now()
		for {
			time.Sleep(10 * time.Second)
			lastModified, errGet := fs.LastModified()
			if errGet != nil {
				panic(errGet)
			}
			if time.Since(lastDumped).Seconds()-time.Since(lastModified).Seconds() > 3 {
				log.Debug("dumping")
				errDump := fs.DumpSQL()
				if errDump != nil {
					panic(errDump)
				}
				lastDumped = time.Now()
			}
		}
	}()
	log.Debugf("running on port 8152")
	http.HandleFunc("/", handler)
	return http.ListenAndServe(":8152", nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	err := handle(w, r)
	if err != nil {
		log.Error(err)
	}
	log.Infof("%v %v %v %s", r.RemoteAddr, r.Method, r.URL, time.Since(t))
}

func handleSearch(w http.ResponseWriter, r *http.Request, domain, query string) (err error) {
	if !isSignedIn(w, r, domain) {
		return handleMain(w, r, domain, "need to log in to search")
	}
	files, errGet := fs.Find(query, domain)
	if errGet != nil {
		return errGet
	}
	return handleList(w, r, domain, query, files)
}

func handleList(w http.ResponseWriter, r *http.Request, domain string, query string, files []db.File) (err error) {
	return listTemplate.Execute(w, TemplateRender{
		Title:      query + " pages",
		Domain:     domain,
		Files:      files,
		NumResults: len(files),
		Search:     query,
		RandomUUID: utils.UUID(),
		SignedIn:   isSignedIn(w, r, domain),
	})
}

func isSignedIn(w http.ResponseWriter, r *http.Request, domain string) bool {
	if domain != "" && domain != "public" {
		cookie, err := r.Cookie(domain)
		if err == nil {
			log.Debugf("got cookie %+v", cookie.Value)
			_, key, err := fs.GetDomainFromName(domain)
			log.Debug(domain, key, err)
			if err == nil && cookie.Value != "" && cookie.Value == key && key != "" {
				return true
			}
		}
	} else {
		return true
	}
	return false
}

func handleMain(w http.ResponseWriter, r *http.Request, domain string, message string) (err error) {
	// check if first time user
	cookie, err := r.Cookie("rwtxt-default-domain")
	var showCookieMessage bool
	if err == nil {
		log.Debugf("got cookie %+v", cookie.Value)
	} else {
		expiration := time.Now().Add(365 * 24 * time.Hour)
		cookie := http.Cookie{Name: "rwtxt-default-domain", Value: "public", Expires: expiration}
		http.SetCookie(w, &cookie)
		showCookieMessage = true
	}

	domainid, _, _ := fs.GetDomainFromName(domain)
	files, err := fs.GetTopX(domain, 10)
	return mainTemplate.Execute(w, TemplateRender{
		Title:             "rwtxt",
		Message:           message,
		Domain:            domain,
		RandomUUID:        utils.UUID(),
		SignedIn:          isSignedIn(w, r, domain),
		Files:             files,
		DomainExists:      domainid != 0,
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
	_, err = r.Cookie(domain)
	if err == nil {
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
	domainKey := strings.TrimSpace(r.FormValue("key"))
	if domain == "public" || domain == "" {
		return handleMain(w, r, "public", "")
	}
	if domainKey == "" {
		return handleMain(w, r, "public", "domain key cannot be empty")
	}
	sha_512 := sha512.New()
	sha_512.Write([]byte("rwtxt"))
	sha_512.Write([]byte(domainKey))
	domainKeyHashed := fmt.Sprintf("%x", sha_512.Sum(nil))

	// check if exists
	_, key, err := fs.GetDomainFromName(domain)
	if err == nil {
		// exists make sure that the keys match
		if domainKeyHashed != key {
			return handleMain(w, r, domain, "You did not enter the correct key to edit on this domain.")
		}
	} else {
		// key doesn't exists, create it
		log.Debugf("domain '%s' doesn't exist, creating it with %s", domain, domainKeyHashed)
		err = fs.SetDomain(domain, domainKeyHashed)
		if err != nil {
			log.Error(err)
		}
	}

	// set domain password
	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: domain, Value: domainKeyHashed, Expires: expiration}
	http.SetCookie(w, &cookie)
	// set domain default
	cookie2 := http.Cookie{Name: "rwtxt-default-domain", Value: domain, Expires: expiration}
	http.SetCookie(w, &cookie2)

	http.Redirect(w, r, "/"+domain, 302)

	return nil
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
		log.Debugf("recv: %v", p)

		if !domainChecked {
			domainChecked = true
			if p.Domain == "public" {
				domainValidated = true
			} else {
				_, key, _ := fs.GetDomainFromName(p.Domain)
				if key != "" && p.DomainKey == key {
					domainValidated = true
				}
			}
		}

		// check profanity
		profane, _, _ := swearChecker.Scorecard(p.Data)

		// save it
		if p.ID != "" && domainValidated && !profane {
			log.Debug("saving")
			if p.Domain == "" {
				p.Domain = "public"
			}
			data := strings.TrimSpace(p.Data)
			log.Debug(data, introText)
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
	} else if strings.HasSuffix(page, "dropzone.css") {
		b, _ := Asset("dropzone.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "rwtxt.css") {
		b, _ := Asset("rwtxt.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
	} else if strings.HasSuffix(page, "dropzone.js") {
		b, _ := Asset("dropzone.js")
		w.Header().Set("Content-Type", "text/javascript")
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
	if havePage {
		var files []db.File
		files, err = fs.Get(page, domain)
		if err != nil {
			log.Error(err)
			return handleMain(w, r, domain, err.Error())
		}
		if len(files) > 1 {
			for i, fi := range files {
				snippet := fi.Data
				if len(snippet) > 50 {
					snippet = snippet[:50]
				}
				reg, _ := regexp.Compile("[^a-z A-Z0-9]+")
				snippet = strings.Replace(snippet, "\n", " ", -1)
				snippet = strings.TrimSpace(reg.ReplaceAllString(snippet, ""))
				files[i].Data = snippet
			}
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
	cookie, err := r.Cookie(domain)
	domainkey := ""
	if err == nil {
		log.Debugf("got cookie %+v", cookie.Value)
		_, key, errGet := fs.GetDomainFromName(domain)
		if errGet == nil && cookie.Value != "" && cookie.Value == key && key != "" {
			domainkey = cookie.Value
		}
	}
	if f.Data == "" {
		f.Data = introText
	}
	return viewEditTemplate.Execute(w, TemplateRender{
		Page:      page,
		Rendered:  utils.RenderMarkdownToHTML(initialMarkdown),
		File:      f,
		IntroText: template.JS(introText),
		Title:     f.Slug,
		Rows:      len(strings.Split(string(utils.RenderMarkdownToHTML(initialMarkdown)), "\n")) + 1,
		Domain:    domain,
		DomainKey: domainkey,
		SignedIn:  isSignedIn(w, r, domain),
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
	if !isSignedIn(w, r, domain) || domain == "public" {
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
			// domain exists, handle normally
			http.Redirect(w, r, "/"+cookie.Value, 302)
			return
		}
		http.Redirect(w, r, "/public", 302)
	} else if r.URL.Path == "/ws" {
		// special path /ws
		return handleWebsocket(w, r)
	} else if strings.HasPrefix(r.URL.Path, "/static") {
		// special path /static
		return handleStatic(w, r)
	} else if r.URL.Path == "/login" {
		// special path /login
		return handleLogin(w, r)
	} else if r.URL.Path == "/logout" {
		// special path /login
		return handleLogout(w, r)
	} else if r.URL.Path == "/upload" {
		// special path /login
		return handleUpload(w, r)
	} else if strings.HasPrefix(r.URL.Path, "/uploads") {
		// special path /login
		return handleUploads(w, r, page)
	} else if domain != "" && page == "" {
		if r.URL.Query().Get("q") != "" {
			return handleSearch(w, r, domain, r.URL.Query().Get("q"))
		}
		// check to see if domain exists
		cookie, cookieErr := r.Cookie("rwtxt-default-domain")
		domainid, _, _ := fs.GetDomainFromName(domain)
		if domainid > 0 || cookieErr != nil {
			// domain exists, handle normally
			return handleMain(w, r, domain, "")
		}
		// we are trying to goto a page that doesn't exist as a domain
		// automatically create a new page for editing in the default domain
		http.Redirect(w, r, "/"+cookie.Value+"/"+domain+"?edit=1", 302)
		return
	} else if domain != "" && page != "" {
		log.Debug("handle view edit")
		return handleViewEdit(w, r, domain, page)
	}
	return
}

// SetLogLevel determines the log level
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

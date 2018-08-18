package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/websocket"
	"github.com/schollz/cowyo2/src/db"
	"github.com/schollz/cowyo2/src/utils"
)

const (
	introText = "This note is empty. Click to edit it."
)

var viewEditTemplate *template.Template
var mainTemplate *template.Template
var loginTemplate *template.Template
var fs *db.FileSystem

type TemplateRender struct {
	Title      string
	Page       string
	Rendered   template.HTML
	File       db.File
	IntroText  template.JS
	Rows       int
	RandomUUID string
	Domain     string
	DomainID   int
	DomainPass string
	SignedIn   bool
}

func init() {
	var err error
	b, err := ioutil.ReadFile("templates/viewedit.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(template.New("viewedit").Parse(string(b)))
	b, err = ioutil.ReadFile("templates/header.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))
	b, err = ioutil.ReadFile("templates/footer.html")
	if err != nil {
		panic(err)
	}
	viewEditTemplate = template.Must(viewEditTemplate.Parse(string(b)))

	b, err = ioutil.ReadFile("templates/main.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(template.New("main").Parse(string(b)))
	b, err = ioutil.ReadFile("templates/header.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))
	b, err = ioutil.ReadFile("templates/footer.html")
	if err != nil {
		panic(err)
	}
	mainTemplate = template.Must(mainTemplate.Parse(string(b)))

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
	ID         string `json:"id,omitempty"`
	DomainID   int    `json:"domainid,omitempty"`
	DomainPass string `json:"domainpass,omitempty"`
	Data       string `json:"data,omitempty"`
	Slug       string `json:"slug,omitempty"`
	Message    string `json:"message,omitempty"`
	Success    bool   `json:"success"`
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serve() (err error) {
	fs, err = db.New("cowyo2.db")
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
	files, errGet := fs.Find(query)
	if errGet != nil {
		return errGet
	}
	initialMarkdown := fmt.Sprintf("<a href='/%s' class='fr'>New</a>\n\n# Found %d '%s'\n\n", utils.UUID(), len(files), query)
	for _, fi := range files {
		snippet := fi.Data
		if len(snippet) > 50 {
			snippet = snippet[:50]
		}
		reg, _ := regexp.Compile("[^a-z A-Z0-9]+")
		snippet = strings.Replace(snippet, "\n", " ", -1)
		snippet = strings.TrimSpace(reg.ReplaceAllString(snippet, ""))
		initialMarkdown += fmt.Sprintf("\n\n(%s) [%s](/%s) *%s*.", fi.Modified.Format("Mon Jan 2 3:04pm 2006"), fi.ID, fi.ID, snippet)
	}
	return viewEditTemplate.Execute(w, TemplateRender{
		Title:    query + " pages",
		Page:     query,
		Rendered: utils.RenderMarkdownToHTML(initialMarkdown),
	})
}

func handleMain(w http.ResponseWriter, r *http.Request, domain string) (err error) {
	return mainTemplate.Execute(w, TemplateRender{
		Title:      "cowyo2",
		Domain:     domain,
		RandomUUID: utils.UUID(),
		SignedIn:   false || domain == "public",
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) (err error) {
	domain := r.FormValue("domain")
	domainKey := r.FormValue("key")
	if domain != "" && domainKey != "" {
		// sign in
	}
	log.Debug(domain, domainKey)
	return mainTemplate.Execute(w, TemplateRender{
		Title:      "cowyo2",
		RandomUUID: utils.UUID(),
		Domain:     domain,
		SignedIn:   false,
	})
}

func handleWebsocket(w http.ResponseWriter, r *http.Request) (err error) {

	cookie, err := r.Cookie("username")
	log.Debugf("getting cookie: [%+v], %+v", cookie, err)
	// handle websockets on this page
	c, errUpgrade := wsupgrader.Upgrade(w, r, nil)
	if errUpgrade != nil {
		return errUpgrade
	}
	defer c.Close()
	var p Payload
	for {
		err := c.ReadJSON(&p)
		if err != nil {
			log.Debug("read:", err)
			break
		}
		log.Debugf("recv: %v", p)

		// save it
		if p.ID != "" {
			data := strings.TrimSpace(p.Data)
			if data == introText {
				data = ""
			}
			err = fs.Save(db.File{
				ID:      p.ID,
				Slug:    p.Slug,
				Data:    data,
				Created: time.Now(),
			})
			if err != nil {
				log.Debug(err)
			}
			fs, _ := fs.Get(p.Slug)
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
		}
	}
	return
}

func handleStatic(w http.ResponseWriter, r *http.Request) (err error) {
	page := r.URL.Path
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Cache-Control", "public, max-age=7776000")
	// cg.Writer.Header().Set("Content-Encoding", "gzip")
	log.Debug(page)
	if strings.HasSuffix(page, "cowyo2.js") {
		b, _ := ioutil.ReadFile("static/js/cowyo2.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(b)
		return
	} else if strings.HasSuffix(page, "cowyo2.css") {
		b, _ := ioutil.ReadFile("static/css/cowyo2.css")
		w.Header().Set("Content-Type", "text/css")
		w.Write(b)
		return
	}
	return
}

func handleViewEdit(w http.ResponseWriter, r *http.Request, domain, page string) (err error) {

	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: "username", Value: "astaxie", Expires: expiration}
	http.SetCookie(w, &cookie)
	log.Debug("setting cookie")
	// handle new page
	// get edit url parameter
	log.Debugf("loading %s", page)
	havePage, _ := fs.Exists(page)
	initialMarkdown := "<a href='#' id='editlink' class='fr'>Edit</a>"
	var f db.File
	if havePage {
		var files []db.File
		files, err = fs.Get(page)
		if err != nil {
			log.Error(err)
		}
		if len(files) > 1 {
			initialMarkdown = fmt.Sprintf("<a href='/%s/%s' class='fr'>New</a>\n\n# Found %d '%s'\n\n", domain, utils.UUID(), len(files), page)
			for _, fi := range files {
				snippet := fi.Data
				if len(snippet) > 50 {
					snippet = snippet[:50]
				}
				reg, _ := regexp.Compile("[^a-z A-Z0-9]+")
				snippet = strings.Replace(snippet, "\n", " ", -1)
				snippet = strings.TrimSpace(reg.ReplaceAllString(snippet, ""))
				initialMarkdown += fmt.Sprintf("\n\n(%s) [%s](/%s) *%s*.", fi.Modified.Format("Mon Jan 2 3:04pm 2006"), fi.ID, fi.ID, snippet)
			}
			viewEditTemplate.Execute(w, TemplateRender{
				Title:    page + " pages",
				Page:     page,
				Rendered: utils.RenderMarkdownToHTML(initialMarkdown),
			})
			return
		} else {
			f = files[0]
		}
	} else {
		f = db.File{
			ID:       utils.UUID(),
			Created:  time.Now(),
			Modified: time.Now(),
		}
		f.Slug = page
		f.Data = introText
		err = fs.Save(f)
		if err != nil {
			log.Error(err)
		}
		http.Redirect(w, r, "/"+domain+"/"+page+"?edit=1", 302)
	}
	initialMarkdown += "\n\n" + f.Data
	domainid, err := fs.GetDomainID(domain)
	if err != nil {
		return
	}
	return viewEditTemplate.Execute(w, TemplateRender{
		Page:      page,
		Rendered:  utils.RenderMarkdownToHTML(initialMarkdown),
		File:      f,
		IntroText: template.JS(introText),
		Title:     f.Slug,
		Rows:      len(strings.Split(string(utils.RenderMarkdownToHTML(initialMarkdown)), "\n")) + 1,
		Domain:    domain,
		DomainID:  domainid,
	})

}

func handle(w http.ResponseWriter, r *http.Request) (err error) {
	fields := strings.Split(r.URL.Path, "/")
	domain := "public"
	page := ""
	if len(fields) > 2 {
		page = strings.ToLower(fields[2])
	}
	if len(fields) > 1 {
		domain = strings.ToLower(fields[1])
	}
	if r.URL.Path == "/" {
		// special path /
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
	} else if domain != "" && page == "" {
		if r.URL.Query().Get("q") != "" {
			return handleSearch(w, r, domain, r.URL.Query().Get("q"))
		}
		return handleMain(w, r, domain)
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

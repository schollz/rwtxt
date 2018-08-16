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
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/schollz/cowyo2/src/db"
	"github.com/schollz/cowyo2/src/utils"
)

const (
	introText = "This note is empty. Click to edit it."
)

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
	ID      string `json:"id,omitempty"`
	Data    string `json:"data,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success"`
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serve() (err error) {
	fs, err := db.New("cowyo2.db")
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

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// Standardize logs
	r.Use(middleWareHandler(), gin.Recovery())
	// r.HTMLRender = loadTemplates("index.html")
	r.LoadHTMLGlob("templates/*")
	r.GET("/*page", func(cg *gin.Context) {
		page := cg.Param("page")
		log.Debug(page)
		if page == "/" {
			query := cg.DefaultQuery("q", "")
			if query != "" {
				files, err := fs.Find(query)
				if err != nil {
					log.Error(err)
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
				cg.HTML(http.StatusOK, "index.html", gin.H{
					"Title":    query + " pages",
					"Page":     query,
					"Rendered": utils.RenderMarkdownToHTML(initialMarkdown),
				})
				return
			}
			cg.HTML(http.StatusOK, "index.html", gin.H{
				"Rendered": utils.RenderMarkdownToHTML(fmt.Sprintf(`
<a href='/%s' class='fr'>New</a>

# cowyo2 

The simplest way to take notes.
				`, strings.ToLower(utils.UUID()))),
			})
		} else if page == "/ws" {
			// handle websockets on this page
			c, err := wsupgrader.Upgrade(cg.Writer, cg.Request, nil)
			if err != nil {
				log.Debug("upgrade:", err)
				return
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
		} else if strings.HasPrefix(page, "/static") {
			log.Debug(page)
			if strings.HasSuffix(page, "cowyo2.js") {
				b, _ := ioutil.ReadFile("static/js/cowyo2.js")
				cg.Data(200, "text/javascript", b)
				return
			} else if strings.HasSuffix(page, "cowyo2.css") {
				b, _ := ioutil.ReadFile("static/css/cowyo2.css")
				cg.Data(200, "text/css", b)
				return
			}
			return
		} else {
			// handle new page
			// get edit url parameter
			page = page[1:]
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
					initialMarkdown = fmt.Sprintf("<a href='/%s' class='fr'>New</a>\n\n# Found %d '%s'\n\n", utils.UUID(), len(files), page)
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
					cg.HTML(http.StatusOK, "index.html", gin.H{
						"Title":    page + " pages",
						"Page":     page,
						"Rendered": utils.RenderMarkdownToHTML(initialMarkdown),
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
				cg.Redirect(302, "/"+page+"?edit=1")
			}
			initialMarkdown += "\n\n" + f.Data

			cg.HTML(http.StatusOK, "index.html", gin.H{
				"Page":      page,
				"Rendered":  utils.RenderMarkdownToHTML(initialMarkdown),
				"File":      f,
				"IntroText": template.JS(introText),
				"Title":     f.Slug,
			})
		}
	})
	log.Debugf("running on port 8152")
	r.Run(":8152") // listen and serve on 0.0.0.0:8080
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

func addCORS(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("Access-Control-Max-Age", "86400")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "GET")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Max")
	c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
}

func middleWareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		// Add base headers
		addCORS(c)
		// Run next function
		c.Next()
		// Log request
		log.Infof("%v %v %v %s", c.Request.RemoteAddr, c.Request.Method, c.Request.URL, time.Since(t))
	}
}

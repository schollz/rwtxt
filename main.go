package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/schollz/cowyo2/src/db"
	"github.com/schollz/cowyo2/src/utils"
)

const (
	introText = ""
)

func main() {
	serve()
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
		log.Fatal(err)
		return
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.GET("/", func(cg *gin.Context) {
		query := cg.DefaultQuery("q", "")
		if query != "" {
			files, err := fs.Find(query)
			if err != nil {
				log.Fatal(err)
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
			return
		}
		cg.HTML(http.StatusOK, "index.html", gin.H{
			"Rendered": utils.RenderMarkdownToHTML(fmt.Sprintf(`

<a href='/%s' class='fr'>New</a>

# cowyo2 

The simplest way to take notes.
			`, strings.ToLower(utils.UUID()))),
		})
	})
	r.GET("/:page", func(cg *gin.Context) {
		page := cg.Param("page")
		if page == "ws" {
			// handle websockets on this page
			c, err := wsupgrader.Upgrade(cg.Writer, cg.Request, nil)
			if err != nil {
				log.Print("upgrade:", err)
				return
			}
			defer c.Close()
			var p Payload
			for {
				err := c.ReadJSON(&p)
				if err != nil {
					log.Println("read:", err)
					break
				}
				log.Printf("recv: %v", p)

				// save it
				if p.ID != "" {
					err = fs.Save(db.File{
						ID:      p.ID,
						Slug:    p.Slug,
						Data:    strings.TrimSpace(p.Data),
						Created: time.Now(),
					})
					if err != nil {
						log.Println(err)
					}
					fs, _ := fs.Get(p.Slug)
					err = c.WriteJSON(Payload{
						ID:      p.ID,
						Slug:    p.Slug,
						Message: "unique_slug",
						Success: len(fs) < 2,
					})
					if err != nil {
						log.Println("write:", err)
						break
					}
				}
			}
		} else {
			// handle new page
			// get edit url parameter
			log.Printf("loading %s", page)
			havePage, _ := fs.Exists(page)
			initialMarkdown := "<a href='#' id='editlink' class='fr'>Edit</a>"
			var f db.File
			if havePage {
				var files []db.File
				files, err = fs.Get(page)
				if err != nil {
					log.Fatal(err)
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
					log.Fatal(err)
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
	log.Printf("running on port 8152")
	r.Run(":8152") // listen and serve on 0.0.0.0:8080
	return
}

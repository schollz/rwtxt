package utils

import (
	"html/template"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/teris-io/shortid"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

func UUID() string {
	sid, err := shortid.New(1, shortid.DefaultABC, uint64(time.Now().Nanosecond()))
	if err != nil {
		panic(err)
	}
	s, err := sid.Generate()
	if err != nil {
		panic(err)
	}
	return s
}

func RenderMarkdownToHTML(markdown string) template.HTML {
	html := string(blackfriday.Run([]byte(markdown)))
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("class").OnElements("a")
	p.AllowElements("p")
	html = p.Sanitize(html)

	return template.HTML(html)
}

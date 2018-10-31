package rwtxt

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/schollz/rwtxt/pkg/db"
	"github.com/schollz/rwtxt/pkg/utils"
)

const introText = "This note is empty. Click to edit it."

var languageCSS map[string]string

type TemplateRender struct {
	Title              string
	Page               string
	Rendered           template.HTML
	File               db.File
	IntroText          template.JS
	Rows               int
	RandomUUID         string
	Domain             string
	DomainID           int
	DomainKey          string
	DomainIsPrivate    bool
	PrivateEnvironment bool
	DomainValue        template.HTMLAttr
	DomainList         []string
	DomainKeys         map[string]string
	DefaultDomain      string
	SignedIn           bool
	Message            string
	NumResults         int
	Files              []db.File
	MostActiveList     []db.File
	SimilarFiles       []db.File
	Search             string
	DomainExists       bool
	ShowCookieMessage  bool
	EditOnly           bool
	Languages          []string
	LanguageJS         []template.JS
	rwt                *RWTxt
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

func init() {
	b, err := Asset("assets/js/languages.js.gz")
	if err != nil {
		panic(err)
	}
	b2 := bytes.NewBuffer(b)
	var r io.Reader
	r, err = gzip.NewReader(b2)
	if err != nil {
		panic(err)
	}
	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		panic(err)
	}

	languageCSS = make(map[string]string)
	currentLanguage := ""
	for _, line := range strings.Split(string(resB.Bytes()), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "Prism.languages.") {
			language := strings.TrimPrefix(strings.Split(line, "=")[0], "Prism.languages.")
			if len(language) < 30 {
				currentLanguage = language
			}
		}
		if currentLanguage != "" {
			if _, ok := languageCSS[currentLanguage]; !ok {
				languageCSS[currentLanguage] = ""
			}
			languageCSS[currentLanguage] += line + "\n"
		}
	}
}

func NewTemplateRender(rwt *RWTxt) *TemplateRender {
	tr := &TemplateRender{
		rwt: rwt,
	}
	return tr
}

func (tr *TemplateRender) handleSearch(w http.ResponseWriter, r *http.Request, domain, query string) (err error) {
	_, ispublic, _ := tr.rwt.fs.GetDomainFromName(domain)
	if !tr.SignedIn && !ispublic {
		return tr.handleMain(w, r, "need to log in to search")
	}
	files, errGet := tr.rwt.fs.Find(query, tr.Domain)
	if errGet != nil {
		return errGet
	}
	return tr.handleList(w, r, query, files)
}

func (tr *TemplateRender) handleList(w http.ResponseWriter, r *http.Request, query string, files []db.File) (err error) {
	_, ispublic, _ := tr.rwt.fs.GetDomainFromName(tr.Domain)
	if !tr.SignedIn && !ispublic {
		return tr.handleMain(w, r, "need to log in to list")
	}

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
	return tr.rwt.listTemplate.Execute(gz, tr)
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
			err := tr.rwt.fs.Save(newFile)
			if err != nil {
				log.Debug(err)
			}
		}()
	}()
	tr.RandomUUID = newFile.ID

	// delete this
	_, ispublic, domainErr := tr.rwt.fs.GetDomainFromName(tr.Domain)
	signedin := tr.SignedIn
	if domainErr != nil {
		// domain does NOT exist
		signedin = false
	}
	tr.SignedIn = signedin
	tr.DomainIsPrivate = !ispublic && (tr.Domain != "public" || tr.rwt.config.Private)
	tr.PrivateEnvironment = tr.rwt.config.Private
	tr.DomainExists = domainErr == nil
	tr.Files, err = tr.rwt.fs.GetTopX(tr.Domain, 10)
	if err != nil {
		log.Debug(err)
	}

	tr.MostActiveList, _ = tr.rwt.fs.GetTopXMostViews(tr.Domain, 10)
	tr.Title = tr.Domain
	tr.Message = message
	tr.DomainValue = template.HTMLAttr(`value="` + tr.Domain + `"`)

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	return tr.rwt.mainTemplate.Execute(gz, tr)
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
	_, _, err = tr.rwt.fs.GetDomainFromName(tr.Domain)
	if err != nil {
		// domain doesn't exist, create it
		log.Debugf("domain '%s' doesn't exist, creating it", tr.Domain)
		err = tr.rwt.fs.SetDomain(tr.Domain, password)
		if err != nil {
			log.Error(err)
			tr.Domain = "public"
			return tr.handleMain(w, r, err.Error())
		}
	}
	tr.DomainKey, err = tr.rwt.fs.SetKey(tr.Domain, password)
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
	domainFound, err := tr.rwt.fs.CheckKey(tr.DomainKey)
	if err != nil || tr.Domain != domainFound {
		if err != nil {
			log.Debug(err)
		}
		return tr.handleMain(w, r, err.Error())
	}

	err = tr.rwt.fs.UpdateDomain(tr.Domain, password, isPublic)
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
	c, errUpgrade := tr.rwt.wsupgrader.Upgrade(w, r, nil)
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
					err = tr.rwt.addSimilar(editFile.Domain, editFile.ID)
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
				_, keyErr := tr.rwt.fs.CheckKey(p.DomainKey)
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
			err = tr.rwt.fs.Save(editFile)
			if err != nil {
				log.Error(err)
			}
			fs, _ := tr.rwt.fs.Get(p.Slug, p.Domain)

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

func (tr *TemplateRender) handleViewEdit(w http.ResponseWriter, r *http.Request) (err error) {
	// handle new page
	// get edit url parameter
	log.Debugf("loading %s", tr.Page)
	havePage, err := tr.rwt.fs.Exists(tr.Page, tr.Domain)
	if err != nil {
		return
	}
	initialMarkdown := ""
	var f db.File

	// check if domain is public and exists
	_, ispublic, errGet := tr.rwt.fs.GetDomainFromName(tr.Domain)
	if errGet == nil && !tr.SignedIn && !ispublic {
		return tr.handleMain(w, r, "domain is not public, sign in first")
	}

	if havePage {
		var files []db.File
		files, err = tr.rwt.fs.Get(tr.Page, tr.Domain)
		if err != nil {
			log.Error(err)
			return tr.handleMain(w, r, err.Error())
		}
		if len(files) > 1 {
			return tr.handleList(w, r, tr.Page, files)
		} else {
			f = files[0]
		}
		tr.SimilarFiles, err = tr.rwt.fs.GetSimilar(f.ID)
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
		err = tr.rwt.fs.Save(f)
		if err != nil {
			return tr.handleMain(w, r, "domain does not exist")
		}
		log.Debugf("saved: %+v", f)
		http.Redirect(w, r, "/"+tr.Domain+"/"+tr.Page, 302)
		return
	}
	tr.File = f

	// get a specific version
	version := r.URL.Query().Get("version")
	if version != "" {
		versionNum, numErr := strconv.Atoi(version)
		if numErr == nil {
			versionData, versionErr := f.History.GetPreviousByTimestamp(int64(versionNum))
			if versionErr == nil {
				f.Data = versionData
				// prevent editing
				tr.DomainKey = ""
				tr.SignedIn = false
				tr.File.Modified = time.Unix(0, int64(versionNum))
			}
		}
	}

	initialMarkdown += "\n\n" + f.Data
	// if f.Data == "" {
	// 	f.Data = introText
	// }
	// update the view count
	go func() {
		err := tr.rwt.fs.UpdateViews(f)
		if err != nil {
			log.Error(err)
		}
	}()

	// make title
	domain := tr.Domain
	slug := f.Slug
	if domain == "" {
		domain = "public"
	}
	if slug == "" {
		slug = f.ID
	}
	tr.Title = slug + " | " + domain

	tr.Rendered = utils.RenderMarkdownToHTML(initialMarkdown)
	tr.IntroText = template.JS(introText)
	tr.Rows = len(strings.Split(string(utils.RenderMarkdownToHTML(initialMarkdown)), "\n")) + 1
	tr.EditOnly = strings.TrimSpace(f.Data) == ""
	tr.Languages = utils.DetectMarkdownCodeBlockLanguages(initialMarkdown)

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	log.Debug(strings.TrimSpace(f.Data))

	return tr.rwt.viewEditTemplate.Execute(gz, tr)

}

func (tr *TemplateRender) handleUploads(w http.ResponseWriter, r *http.Request, id string) (err error) {
	log.Debug("getting ", id)
	name, data, _, err := tr.rwt.fs.GetBlob(id)
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
	// special check for sign in
	for _, domainName := range tr.DomainList {
		if domain == domainName {
			tr.SignedIn = true
			break
		}
	}
	if !tr.SignedIn || domain == "public" {
		log.Debugf("got domain: %s, signed in: %+v", domain, tr)
		log.Debugf("refusing to upload")
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
	err = tr.rwt.fs.SaveBlob(id, info.Filename, fileData.Bytes())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", "/uploads/"+id+"?filename="+url.QueryEscape(info.Filename))
	_, err = w.Write([]byte("ok"))
	return
}

func (tr *TemplateRender) handleExport(w http.ResponseWriter, r *http.Request) (err error) {
	log.Debug("exporting")
	if tr.Domain == "public" {
		return tr.handleMain(w, r, "can't export public")
	}
	if !tr.SignedIn {
		return tr.handleMain(w, r, "must sign in")
	}
	files, _ := tr.rwt.fs.GetAll(tr.Domain)
	for i := range files {
		files[i].DataHTML = template.HTML("")
	}
	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(files)
	w.Write(js)
	return
}

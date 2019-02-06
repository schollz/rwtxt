package utils

import (
	"archive/zip"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"html/template"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/crypto/bcrypt"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

// ZipFiles will zip files to filename
func ZipFiles(filename string, files []string) error {

	newZipFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	// Add files to zip
	for _, file := range files {

		zipfile, err := os.Open(file)
		if err != nil {
			return err
		}
		defer zipfile.Close()

		// Get the file information
		info, err := zipfile.Stat()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Using FileInfoHeader() above only uses the basename of the file. If we want
		// to preserve the folder structure we can overwrite this with the full path.
		header.Name = file

		// Change to deflate to gain better compression
		// see http://golang.org/pkg/archive/zip/#pkg-constants
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err = io.Copy(writer, zipfile); err != nil {
			return err
		}
	}
	return nil
}
func RenderMarkdownToHTML(markdown string) template.HTML {
	html := string(blackfriday.Run([]byte(markdown),
		blackfriday.WithExtensions(
			blackfriday.Autolink|
				blackfriday.Strikethrough|
				blackfriday.SpaceHeadings|
				blackfriday.BackslashLineBreak|
				blackfriday.NoIntraEmphasis|
				blackfriday.Tables|
				blackfriday.FencedCode|
				blackfriday.AutoHeadingIDs|
				blackfriday.Footnotes),
	))

	p := bluemonday.UGCPolicy()
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("class").OnElements("a")
	p.AllowAttrs("style").OnElements("span")
	p.AllowAttrs("class").OnElements("code")
	p.AllowElements("p")
	html = p.Sanitize(html)

	return template.HTML(html)
}

var src = rand.NewSource(time.Now().UTC().UnixNano())

const letterBytes = "abcdefghijklmnopqrstuvwxyz0123456789"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func UUID() string {
	n := 10
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

// Hash generates a hash of data using HMAC-SHA-512/256. The tag is intended to
// be a natural-language string describing the purpose of the hash, such as
// "hash file for lookup key" or "master secret to client secret".  It serves
// as an HMAC "key" and ensures that different purposes will have different
// hash output. This function is NOT suitable for hashing passwords.
func Hash(tag string, data string) string {
	h := hmac.New(sha512.New512_256, []byte(tag))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// HashPassword generates a bcrypt hash of the password using work factor 10.
func HashPassword(password string) (string, error) {
	passB, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return hex.EncodeToString(passB), err
}

// CheckPasswordHash securely compares a bcrypt hashed password with its possible
// plaintext equivalent.  Returns nil on success, or an error on failure.
func CheckPasswordHash(hash, password string) error {
	hashB, err := hex.DecodeString(hash)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword(hashB, []byte(password))
}

// DetectMarkdownCodeBlockLanguages detects and returns programming languages used in
// code blocks
func DetectMarkdownCodeBlockLanguages(markdown string) (codeblocks []string) {
	codeblocksMap := make(map[string]struct{})
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if len(line) <= 3 {
			continue
		}
		if !strings.HasPrefix(line, "```") {
			continue
		}
		codeblocksMap[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "```")))] = struct{}{}
	}
	codeblocks = make([]string, len(codeblocksMap))
	i := 0
	for key := range codeblocksMap {
		codeblocks[i] = key
		i++
	}
	return
}

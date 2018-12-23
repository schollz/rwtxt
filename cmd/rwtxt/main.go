package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	log "github.com/cihub/seelog"
	_ "github.com/mattn/go-sqlite3"
	"github.com/schollz/rwtxt"
	"github.com/schollz/rwtxt/pkg/db"
)

var (
	dbName  string
	Version string
)

func main() {
	var (
		err             error
		export          = flag.Bool("export", false, "export uploads to {{TIMESTAMP}}-uploads.zip and posts to {{TIMESTAMP}}-posts.zip")
		resizeWidth     = flag.Int("resizewidth", -1, "image width to resize on the fly")
		resizeOnUpload  = flag.Bool("resizeonupload", false, "resize on upload")
		resizeOnRequest = flag.Bool("resizeonrequest", false, "resize on request")
		debug           = flag.Bool("debug", false, "debug mode")
		showVersion     = flag.Bool("v", false, "show version")
		profileMemory   = flag.Bool("memprofile", false, "profile memory")
		database        = flag.String("db", "rwtxt.db", "name of the database")
		listen          = flag.String("listen", ":8152", "interface:port to listen on")
		private         = flag.Bool("private", false, "private setup (allows listing of public notes)")
		created         = flag.Bool("created", false, "order by date created rather than date modified")
	)
	flag.Parse()

	if *profileMemory {
		go func() {
			for {
				time.Sleep(30 * time.Second)
				log.Info("writing memprofile")
				f, err := os.Create("memprofile")
				if err != nil {
					panic(err)
				}
				pprof.WriteHeapProfile(f)
				f.Close()
			}
		}()
	}

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

	fs, err := db.New(dbName)
	if err != nil {
		panic(err)
	}

	if *export {
		err = fs.ExportPosts()
		if err != nil {
			panic(err)
		}
		err = fs.ExportUploads()
		if err != nil {
			panic(err)
		}
		return
	}

	config := rwtxt.Config{
		Bind:            *listen,
		Private:         *private,
		ResizeWidth:     *resizeWidth,
		ResizeOnRequest: *resizeOnRequest,
		ResizeOnUpload:  *resizeOnUpload,
		OrderByCreated:  *created,
	}

	rwt, err := rwtxt.New(fs, config)
	if err != nil {
		panic(err)
	}

	err = rwt.Serve()
	if err != nil {
		log.Error(err)
	}
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

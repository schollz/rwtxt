HASH=$(shell git describe --always)
LDFLAGS=-ldflags "-s -w -X main.Version=${HASH}"

prereq: 
	go get -d -v github.com/tdewolff/minify/...
	go install -v github.com/tdewolff/minify/cmd/minify
	go get -d -v github.com/jteeuwen/go-bindata/...
	go install -v github.com/jteeuwen/go-bindata/go-bindata

bundle:
	rm -rf assets
	cp -r static assets
	cd assets && gzip -9 -r *
	cp templates/main.html assets/main.html
	cp templates/footer.html assets/footer.html
	cp templates/list.html assets/list.html
	cp templates/header.html assets/header.html
	cp templates/viewedit.html assets/viewedit.html
	cp templates/prism.js assets/prism.js

exec: prereq bundle
	go-bindata -pkg rwtxt -nocompress assets assets/img assets/js assets/css assets/img/favicon
	cd cmd/rwtxt && go build -v --tags "fts4" ${LDFLAGS} && cp rwtxt ../../

quick: bundle
	go-bindata -pkg rwtxt -nocompress assets assets/img assets/js assets/css assets/img/favicon
	cd cmd/rwtxt && go build -v --tags "fts4" ${LDFLAGS} && cp rwtxt ../../

run: build
	$(GOPATH)/bin/rwtxt

debug: 
	go get -v --tags "fts4" ${LDFLAGS} ./...
	$(GOPATH)/bin/rwtxt --debug

dev:
	rerun make run

release:
	docker pull karalabe/xgo-latest
	go get github.com/karalabe/xgo
	mkdir -p bin
	xgo -go $(shell go version) -dest bin ${LDFLAGS} -targets linux/amd64,linux/arm-6,darwin/amd64,windows/amd64 github.com/schollz/rwtxt/cmd/rwtxt
	# cd bin && upx --brute kiki-linux-amd64

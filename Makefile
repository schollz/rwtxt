build:
	go get -v github.com/tdewolff/minify/...
	go get -v github.com/jteeuwen/go-bindata/...
	rm -rf assets 
	mkdir assets
	cp templates/main.html assets/main.html
	cp templates/footer.html assets/footer.html
	cp templates/list.html assets/list.html
	cp templates/header.html assets/header.html
	cp templates/viewedit.html assets/viewedit.html
	minify static/css/rwtxt.css | gzip -9   > assets/rwtxt.css
	minify static/css/dropzone.css | gzip -9  > assets/dropzone.css
	minify static/js/rwtxt.js | gzip -9  > assets/rwtxt.js
	minify static/js/dropzone.js | gzip -9 > assets/dropzone.js
	minify static/css/prism.css | gzip -9 > assets/prism.css
	minify static/js/prism.js | gzip -9  > assets/prism.js
	gzip -9 -c static/img/logo.png  > assets/logo.png
	cd assets && go-bindata * && cd ..
	mv assets/bindata.go .
	go build -v --tags "fts4"	

run: build
	./rwtxt

dev:
	rerun make run

build:
	rm -rf assets 
	mkdir assets
	cp templates/main.html assets/main.html
	cp templates/footer.html assets/footer.html
	cp templates/list.html assets/list.html
	cp templates/header.html assets/header.html
	cp templates/viewedit.html assets/viewedit.html
	gzip -9 -c static/css/rwio.css > assets/rwio.css
	gzip -9 -c static/css/dropzone.css > assets/dropzone.css
	gzip -9 -c static/js/rwio.js > assets/rwio.js
	gzip -9 -c static/js/dropzone.js > assets/dropzone.js
	cd assets && go-bindata * && cd ..
	mv assets/bindata.go .
	go build -v --tags "fts4"	

run: build
	./rwio

dev:
	rerun make run
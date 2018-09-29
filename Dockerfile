FROM golang:1.11-alpine as builder
RUN apk add --no-cache git make g++ gzip
RUN go get -v github.com/jteeuwen/go-bindata/go-bindata
RUN go get -v github.com/tdewolff/minify/... 
RUN git clone https://github.com/schollz/rwtxt.git 
WORKDIR /go/rwtxt
RUN make

FROM alpine:latest 
VOLUME /data
EXPOSE 8152
COPY --from=builder /go/bin/rwtxt /rwtxt
CMD ["/rwtxt","--db","/data/rwtxt.db"]

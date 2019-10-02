FROM golang:1.13-alpine as builder
RUN apk add --no-cache git make g++ gzip
RUN go get -v github.com/jteeuwen/go-bindata/go-bindata
RUN go get -v github.com/tdewolff/minify/...
WORKDIR /go/rwtxt
COPY . .
RUN make exec

FROM alpine:latest 
VOLUME /data
EXPOSE 8152
COPY --from=builder /go/rwtxt/rwtxt /rwtxt
ENTRYPOINT ["/rwtxt"]
CMD ["--db","/data/rwtxt.db","--resizeonrequest","--resizewidth","600"]

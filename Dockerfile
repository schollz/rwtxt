# First build step
FROM golang:1.11-alpine

#WORKDIR /go/src/github.com/schollz/rwtxt
#COPY . .
# Install git and make, compile and cleanup
RUN apk add --no-cache git make g++ \
    && go get -v github.com/jteeuwen/go-bindata/go-bindata \
    && go get -v github.com/tdewolff/minify/... \
    && git clone https://github.com/schollz/rwtxt.git \
    && cd rwtxt \
    && make \
    && apk del --purge git make g++ \
    && rm -rf /var/cache/apk* \
    && mv /go/rwtxt/rwtxt /rwtxt \
    && rm -rf /go

VOLUME /data
EXPOSE 8142
# Start rwtxt listening on any host
CMD ["/rwtxt","--db","/data/rwtxt.db"]

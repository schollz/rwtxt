# First build step
FROM golang:1.10-alpine as builder

#WORKDIR /go/src/github.com/schollz/rwtxt
#COPY . .
# Enable crosscompiling
ENV CGO_ENABLED=1

# Install git and make, compile and cleanup
RUN apk add --no-cache git make \
    && go get -u -v github.com/jteeuwen/go-bindata/... \
    && go get -u -v -d github.com/schollz/rwtxt \
	&& cd /go/src/github.com/schollz/rwtxt \
    && make \
    && apk del --purge git make \
    && rm -rf /var/cache/apk*

# Second build step uses the minimal scratch Docker image
FROM scratch
# Copy the binary from the first step
COPY --from=builder /go/src/github.com/schollz/rwtxt/rwtxt /usr/local/bin/rwtxt
# Expose data folder
VOLUME /data
EXPOSE 8050
# Start rwtxt listening on any host
CMD ["rwtxt"]
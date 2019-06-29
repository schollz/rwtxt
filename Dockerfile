FROM golang:1.12-alpine as builder
RUN apk add --no-cache git make g++ gzip
WORKDIR /go/rwtxt
COPY . .
RUN make exec

FROM alpine:latest
RUN apk add --no-cache tzdata
VOLUME /data
EXPOSE 8152
COPY --from=builder /go/rwtxt/rwtxt /rwtxt
ENTRYPOINT ["/rwtxt"]
CMD ["--db","/data/rwtxt.db"]

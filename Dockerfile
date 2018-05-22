FROM golang:1.7.4-alpine
RUN apk update
RUN apk add openssl ca-certificates git
RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app
COPY . /usr/src/app
RUN go get "gopkg.in/mcuadros/go-syslog.v2"
RUN go build main.go
CMD ["/usr/src/app/main"]
EXPOSE 9000

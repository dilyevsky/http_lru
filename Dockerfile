FROM golang:1.9-alpine

RUN apk add --update --no-cache git libc-dev

ENV SRC_DIR=/go/src/github.com/dilyevsky/httplru/
WORKDIR $SRC_DIR
ADD . $SRC_DIR

RUN go get -u github.com/golang/dep/cmd/dep

RUN dep ensure
RUN go build -o httplru

CMD ./httplru
EXPOSE 8080

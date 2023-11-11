FROM golang:1-alpine as builder

RUN apk --no-cache --no-progress add make git

WORKDIR /go/gonetmon

ENV GO111MODULE on

# Download go modules
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build -o dist/gnm cmd/gnm/main.go

FROM alpine:3
RUN apk update \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates

COPY --from=builder /go/gonetmon/dist/gnm /usr/bin/gnm

ENTRYPOINT [ "/usr/bin/gnm" ]

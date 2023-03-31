FROM golang:1.20-alpine AS builder
RUN apk update && \
    apk add --no-cache git gcc musl-dev
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN GOOS=linux GOARCH=amd64 go build -o /out/gptbot
RUN go test -v ./...

FROM alpine:latest
WORKDIR /opt/bot
COPY --from=builder /out/ /opt/gptbot/
CMD /opt/gptbot/gptbot

FROM golang:1.25-alpine AS builder
RUN apk update && \
    apk add --no-cache git gcc musl-dev
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN GOOS=linux GOARCH=amd64 go build -o /out/gptbot

FROM alpine:latest
WORKDIR /opt/gptbot
COPY --from=builder /out/ /opt/gptbot/
CMD [ "/opt/gptbot/gptbot", "run", "-v" ]

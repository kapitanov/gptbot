FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN GOOS=linux GOARCH=amd64 go build -o /out/gptbot && \
    /out/gptbot --version

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /opt/gptbot
COPY --from=builder /out/ /opt/gptbot/
CMD [ "/opt/gptbot/gptbot", "run", "-v" ]

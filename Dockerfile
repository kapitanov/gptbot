FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gptbot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /opt/gptbot
COPY --from=builder /app/gptbot .
COPY --from=builder /app/conf ./conf

# Create var directory for storage
RUN mkdir -p ./var

CMD [ "./gptbot" ]


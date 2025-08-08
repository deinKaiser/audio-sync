# Build stage
FROM golang:1.24.4 AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./
COPY static ./static

RUN go get audio-sync 
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/app .
COPY --from=builder /app/static ./static

EXPOSE 8080

CMD ["./app"]
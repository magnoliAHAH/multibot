FROM golang:1.24-alpine

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o bot

CMD ["./bot"]
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o app .

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/app ./app
EXPOSE 8080
ENTRYPOINT ["./app"]

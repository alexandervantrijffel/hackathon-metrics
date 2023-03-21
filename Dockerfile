FROM golang:alpine as builder

WORKDIR /app

COPY go.* ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags="-w -s" -o /app/example ./cmd/example

FROM scratch
WORKDIR /app
COPY --from=builder /app/example example
CMD ["./example"]

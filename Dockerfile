FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /linguaai .

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /linguaai .
COPY --from=builder /app/static ./static
RUN mkdir -p /app/data
EXPOSE 8080
CMD ["/app/linguaai"]

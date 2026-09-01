# --- build stage ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /ambulance-api .

# --- run stage ---
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /ambulance-api .

EXPOSE 8080
CMD ["./ambulance-api"]

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/app .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/app ./app

# All config (DB_HOST, DB_PASSWORD, HTTP_PORT, etc.) comes from Render's env vars at runtime,
# not a baked-in configs/.env — godotenv only fills in vars that aren't already set.
# Render injects PORT; gofr listens on HTTP_PORT, so map one to the other at startup.
CMD ["sh", "-c", "HTTP_PORT=${PORT:-8000} ./app"]

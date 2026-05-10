FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /sila ./cmd/server/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /sila /usr/local/bin/sila
RUN mkdir -p /data
ENV PORT=8080
ENV SILA_DB_PATH=/data/sila.db
ENV SILA_DATA_DIR=/data
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["sila"]

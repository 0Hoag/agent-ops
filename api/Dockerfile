FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/agent-ops-api .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build /out/agent-ops-api /agent-ops-api
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=3s --retries=30 CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["/agent-ops-api"]

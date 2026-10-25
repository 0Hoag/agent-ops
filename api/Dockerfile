FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/agent-ops-api .

FROM alpine:3.22
RUN adduser -D -H app
COPY --from=build /out/agent-ops-api /agent-ops-api
USER app
EXPOSE 8080
ENTRYPOINT ["/agent-ops-api"]

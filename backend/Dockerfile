# syntax=docker/dockerfile:1.7

FROM golang:1.23-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates

# Pull host CA certs from the named build context "host_certs".
COPY --from=host_certs . /usr/local/share/ca-certificates/
RUN update-ca-certificates

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/llm-proxy ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/llm-proxy /llm-proxy
COPY --from=build /etc/ssl/certs/ /etc/ssl/certs/
EXPOSE 8080
ENTRYPOINT ["/llm-proxy"]

FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/llm-proxy ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/llm-proxy /llm-proxy
EXPOSE 8080
ENTRYPOINT ["/llm-proxy"]

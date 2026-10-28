FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/gate ./cmd/gate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/gate /gate
USER nobody
EXPOSE 8080
ENTRYPOINT ["/gate"]

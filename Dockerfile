FROM golang:1.24-alpine AS build
ENV CGO_ENABLED=0
COPY . /app
WORKDIR /app
RUN go build ./...

FROM alpine:latest
WORKDIR /
COPY --from=build /app .
ENTRYPOINT ./metal-seed-mutator

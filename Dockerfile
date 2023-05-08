FROM golang:1.20-alpine AS build
ENV CGO_ENABLED=0
COPY . /app
WORKDIR /app
RUN go build mutator.go

FROM alpine:latest
WORKDIR /
COPY --from=build /app .
ENTRYPOINT ./mutator

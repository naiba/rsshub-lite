FROM golang:alpine AS binarybuilder
WORKDIR /rsshub-lite/
COPY . .
RUN go build -o rsshub-lite cmd/main.go

FROM alpine:latest
RUN echo http://dl-2.alpinelinux.org/alpine/edge/community/ >>/etc/apk/repositories && apk --no-cache --no-progress add \
  tzdata \
  ca-certificates
WORKDIR /rsshub-lite/
COPY views ./views
COPY --from=binarybuilder /rsshub-lite/rsshub-lite .
VOLUME ["/rsshub-lite/data"]
EXPOSE 3000
CMD ["/rsshub-lite/rsshub-lite"]

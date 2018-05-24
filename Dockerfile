FROM golang:alpine as go_builder

RUN apk add --no-cache curl git gcc musl-dev
RUN curl https://glide.sh/get | sh

WORKDIR /go/src/app
COPY glide.* ./
RUN glide install
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -a app .

FROM node:alpine as asset_builder
WORKDIR /app
COPY package* /app/
RUN npm install
COPY assets /app/assets
RUN npm run build

FROM alpine:3.7
WORKDIR /
COPY --from=go_builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go_builder /go/src/app/app /app
COPY templates /templates
COPY --from=asset_builder /app/assets /assets
ENTRYPOINT ["/app"]
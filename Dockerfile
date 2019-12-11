FROM golang:alpine as go_builder

WORKDIR /src
COPY go.* ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o app .

FROM node:alpine as asset_builder
RUN apk add python2
WORKDIR /app
COPY package* /app/
RUN npm install
COPY assets /app/assets
RUN npm run build

FROM scratch
COPY --from=go_builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go_builder /src/app /app
COPY templates /templates
COPY --from=asset_builder /app/assets /assets
ENTRYPOINT ["/app"]

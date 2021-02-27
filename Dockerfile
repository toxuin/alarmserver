FROM golang:1.16-alpine AS build_base
RUN apk add git

WORKDIR /tmp/app

COPY . .
RUN go get -d ./... && go build -o ./out/alarmserver

FROM alpine:3.12
RUN apk add ca-certificates
COPY --from=build_base /tmp/app/out/alarmserver /app/alarmserver

EXPOSE 15002

CMD ["/app/alarmserver"]

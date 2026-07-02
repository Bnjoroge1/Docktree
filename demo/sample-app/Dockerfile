FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /app .

FROM alpine:latest
RUN adduser -D -u 1000 app
WORKDIR /app
COPY version.json /app/version.json
COPY --from=build /app /app/app
USER app
EXPOSE 3000
CMD ["/app/app"]

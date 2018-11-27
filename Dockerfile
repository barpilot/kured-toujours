FROM golang:1.11 as build
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kured-toujours .


FROM alpine:latest
COPY --from=build /app/kured-toujours /bin/kured-toujours
ENTRYPOINT ["/bin/kured-toujours"]

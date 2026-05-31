FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata git
COPY act /usr/local/bin/act
ENTRYPOINT ["/usr/local/bin/act"]

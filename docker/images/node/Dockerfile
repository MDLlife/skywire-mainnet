# Builder
# ARG builder_base=golang:alpine 
ARG base=alpine
FROM golang:alpine  as builder

ARG CGO_ENABLED=0
ENV CGO_ENABLED=${CGO_ENABLED} \
    GOOS=linux  \
    GOARCH=amd64 \
    GO111MODULE=on

COPY . skywire 

WORKDIR skywire

RUN go build -mod=vendor -tags netgo -ldflags="-w -s" \
      -o skywire-visor cmd/skywire-visor/skywire-visor.go &&\
    go build  -mod=vendor -ldflags="-w -s" -o skywire-cli ./cmd/skywire-cli	&&\
    go build  -mod=vendor -ldflags="-w -s" -o ./apps/skychat.v1.0 ./cmd/apps/skychat	&&\
	  go build  -mod=vendor -ldflags="-w -s" -o ./apps/helloworld.v1.0 ./cmd/apps/helloworld &&\
	  go build  -mod=vendor -ldflags="-w -s" -o ./apps/skysocks.v1.0 ./cmd/apps/skysocks &&\
	  go build  -mod=vendor -ldflags="-w -s" -o ./apps/skysocks-client.v1.0  ./cmd/apps/skysocks-client
	

## Resulting image
FROM ${base} as node-runner

COPY --from=builder /go/skywire/skywire-visor skywire-visor
COPY --from=builder /go/skywire/apps bin/apps
COPY --from=builder /go/skywire/docker/images/node/update.sh update.sh
COPY --from=builder /go/skywire/skywire-cli bin/skywire-cli

RUN ./update.sh

ENTRYPOINT [ "./skywire-visor" ]

# default target
FROM  node-runner

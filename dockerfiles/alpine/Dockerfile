FROM golang:alpine as builder
COPY . /go/src/github.com/concourse/pool-resource
ENV CGO_ENABLED 0
ENV GOPATH /go/src/github.com/concourse/pool-resource/Godeps/_workspace:${GOPATH}
ENV PATH /go/src/github.com/concourse/pool-resource/Godeps/_workspace/bin:${PATH}
RUN go build -o /assets/out github.com/concourse/pool-resource/cmd/out
RUN set -e; for pkg in $(go list ./...); do \
		go test -o "/tests/$(basename $pkg).test" -c $pkg; \
	done

FROM concourse/git-resource:alpine AS resource

ADD assets/ /opt/resource/
RUN chmod +x /opt/resource/*

COPY --from=builder /assets /opt/go
RUN chmod +x /opt/go/out

FROM resource AS tests
COPY --from=builder /tests /go/resource-tests/
RUN set -e; for test in /go/resource-tests/*.test; do \
		$test; \
	done

ADD test/ /opt/resource-tests
RUN /opt/resource-tests/all.sh

FROM resource

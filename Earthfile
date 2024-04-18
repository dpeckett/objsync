VERSION 0.7
FROM golang:1.22-bookworm
WORKDIR /workspace

tidy:
  LOCALLY
  RUN go mod tidy
  RUN go fmt ./...

lint:
  FROM golangci/golangci-lint:v1.57.2
  WORKDIR /workspace
  COPY . ./
  RUN golangci-lint run --timeout 5m ./...

test:
  RUN apt update && apt install -y curl jq
  RUN curl -fsSL https://get.docker.com | bash
  COPY +modules/modules /lib/modules
  COPY go.* ./
  RUN go mod download
  COPY . .
  WITH DOCKER --allow-privileged
    RUN --privileged mount -t devtmpfs devtmpfs /dev \
      && go test -coverprofile=coverage.out -v ./...
  END
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out

modules:
  LOCALLY
  SAVE ARTIFACT /lib/modules
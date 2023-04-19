# Build Dependencies ---------------------------
FROM golang:1.19-alpine AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace
COPY go.mod .
COPY go.sum .

RUN go mod download

# Build the app --------------------------------
FROM build_deps AS build

COPY . .
RUN CGO_ENABLED=0 go build -o vault-unsealer -ldflags '-w -extldflags "-static"' .

# Package the image ----------------------------
#FROM scratch
FROM alpine:3.17.3

COPY --from=build /workspace/vault-unsealer /usr/local/bin/vault-unsealer
#ENTRYPOINT ["vault-unsealer"]
ENTRYPOINT ["tail", "-f", "/dev/null"]
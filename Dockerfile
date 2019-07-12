FROM golang:1.12 as build
WORKDIR /go/src/app
ENV GO111MODULE=on
COPY go.* ./
RUN go mod download
COPY ./ ./
RUN go test ./... && go install ./...

# Now copy it into our base image.
FROM gcr.io/distroless/base
COPY --from=build /go/bin/* /

FROM golang:1.26-alpine
WORKDIR /src
COPY hello.go ./
RUN go build -o /out/hello ./hello.go
USER 65532:65532

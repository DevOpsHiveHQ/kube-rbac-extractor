FROM golang:alpine

WORKDIR /app

COPY . /app

RUN go build -o kube-rbac-extractor

ENTRYPOINT [ "./kube-rbac-extractor" ]
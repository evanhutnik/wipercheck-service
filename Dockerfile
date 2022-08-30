FROM golang:1.17-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o /wipercheck-service ./cmd/service

##
## Deploy
##
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=build /wipercheck-service /wipercheck-service

USER nonroot:nonroot

ENTRYPOINT ["/wipercheck-service"]
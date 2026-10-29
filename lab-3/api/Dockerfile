FROM golang:1.27 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/app .

FROM scratch
COPY --from=build /out/app /app
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/app"]

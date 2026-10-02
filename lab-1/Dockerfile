FROM golang:1.27 AS build
WORKDIR /src

COPY api/go.mod ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api .

FROM scratch
COPY --from=build /out/api /api
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/api"]

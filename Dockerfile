# The single dandori binary on a distroless base. The templ code is
# generated and committed, so building needs only Go.
FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /dandori ./cmd/dandori

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /dandori /dandori
ENV DANDORI_ADDR=0.0.0.0:8080
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=3 CMD ["/dandori", "healthcheck"]
ENTRYPOINT ["/dandori"]

# DayTrader Go target. Build context: daytrader-go/
#   docker build -t daytrader-go daytrader-go
# The Go version matches go.mod.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/daytrader ./cmd/daytrader

# Static binary only: no shell, no package manager. Runs as uid 65532 (nonroot).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/daytrader /daytrader
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/daytrader"]
CMD ["serve"]

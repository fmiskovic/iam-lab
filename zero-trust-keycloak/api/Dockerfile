# One image for both APIs; API_NAME selects inventory-api or payment-api.
FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api .

# No shell, no package manager: just the binary, running as a non-root user.
FROM scratch
COPY --from=build /api /api
USER 65532:65532
ENTRYPOINT ["/api"]

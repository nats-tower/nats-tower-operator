
# build backend
FROM golang:1.24-alpine AS build

RUN apk add --no-cache git

WORKDIR /src

COPY ./ ./

# Run tests​
# RUN CGO_ENABLED=0 go test -timeout 30s -v ./...

# Build the executable​
RUN CGO_ENABLED=0 go build \
    -mod=vendor \
    -o /app ./

# STAGE 2: build the container to run​
FROM gcr.io/distroless/static AS final

# copy compiled app​
COPY --from=build /app /app

ENTRYPOINT ["/app"]

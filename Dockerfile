FROM golang:1.24.2 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lantern ./cmd/lantern

FROM gcr.io/distroless/static-debian12:nonroot

ENV HOME=/home/nonroot
WORKDIR /home/nonroot

COPY --from=build /out/lantern /usr/bin/lantern

EXPOSE 4317 4318 4000
VOLUME ["/home/nonroot/.lantern"]

ENTRYPOINT ["/usr/bin/lantern"]
CMD ["serve", "--no-tui", "--db", "/home/nonroot/.lantern/data.db"]

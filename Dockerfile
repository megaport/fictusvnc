## Build image #####################################
FROM golang:1.24 AS build

WORKDIR /workspace/

COPY go.mod go.sum ./
RUN GOPROXY=direct go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-w -s" -o fictusvnc_websocket

## Final image #####################################
FROM scratch AS final
WORKDIR /
COPY --from=build /workspace/fictusvnc_websocket /fictusvnc_websocket
COPY --from=build /workspace/default.png /default.png
EXPOSE 5900
ENTRYPOINT ["/fictusvnc_websocket"]

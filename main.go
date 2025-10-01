// main.go
// Minimal VNC server main entry point and basic utilities.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var (
	serverImage *fb
	serverName  string

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func main() {
	serverNameFlag := flag.String("servername", "Mock VNC server", "Server name")
	portNumberFlag := flag.Int("port", 5900, "VNC port")
	imageNameFlag := flag.String("image", "./images/default.png", "Image to display")
	certFilenameFlag := flag.String("certfile", "./cert.pem", "Certificate file")
	keyFilenameFlag := flag.String("keyfile", "./key.pem", "Key file")
	flag.Parse()

	serverName = *serverNameFlag

	img, err := loadImage(*imageNameFlag)
	if err != nil {
		log.Fatalf("failed to load the image: %s\n", err)
	}
	serverImage = img

	r := chi.NewRouter()
	r.Route("/vnc", func(sub chi.Router) {
		sub.Get("/", serverFunc)
	})
	listenAddress := fmt.Sprintf(":%d", *portNumberFlag)
	server := &http.Server{
		Addr:    listenAddress,
		Handler: r,
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(
		signalChan,
		syscall.SIGINT,  // ctrl^C
		syscall.SIGTERM, // docker shutdown
	)

	go func(ch chan os.Signal) {
		<-ch
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Printf("proxy shutdown error: %s\n", err)
		}
	}(signalChan)

	if err := server.ListenAndServeTLS(*certFilenameFlag, *keyFilenameFlag); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}

	log.Printf("done\n")
}

func serverFunc(w http.ResponseWriter, r *http.Request) {
	log.Printf("incoming connection\n")

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("failed to upgrade incoming request to websocket: %s\n", err)
		http.Error(w, "failed to upgrade incoming connection", http.StatusBadGateway)
		return
	}

	go serveWebsocket(wsConn, serverImage, serverName)
}

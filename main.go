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
	"path"
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
	imageNameFlag := flag.String("image", "default.png", "Image to display")
	certNameFlag := flag.String("certfile", "", "Certificate file")
	keyNameFlag := flag.String("keyfile", "", "Key file")
	flag.Parse()

	serverName = getValue(*serverNameFlag, "FICTUSVNC_SERVER_NAME")
	log.Printf("server name: %s\n", serverName)

	filePath := getValue("", "FILE_PATH")
	log.Printf("file path: %s\n", filePath)

	imageName := getValue(*imageNameFlag, "FICTUSVNC_IMAGE")
	log.Printf("image name: %s\n", imageName)
	imageFileName := path.Join(filePath, imageName)
	log.Printf("full image file name: %s\n", imageFileName)

	certName := getValue(*certNameFlag, "FICTUSVNC_CERTFILE")
	log.Printf("cert name: %s\n", certName)
	certFileName := path.Join(filePath, certName)
	log.Printf("full cert file name: %s\n", certFileName)

	keyName := getValue(*keyNameFlag, "FICTUSVNC_KEYFILE")
	log.Printf("key name: %s\n", keyName)
	keyFileName := path.Join(filePath, keyName)
	log.Printf("full key file name: %s\n", keyFileName)

	img, err := loadImage(imageFileName)
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

	if len(certFileName) > 0 && len(keyFileName) > 0 {
		log.Printf("using TLS\n")
		if err := server.ListenAndServeTLS(certFileName, keyFileName); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	} else {
		log.Printf("using http\n")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}

	log.Printf("done\n")
}

func getValue(value, envVarName string) string {
	envValue := os.Getenv(envVarName)
	if len(envValue) > 0 {
		return envValue
	}
	return value
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

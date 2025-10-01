package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

const (
	rfbVersion              = "RFB 003.008\n"
	msgSetPixelFormat       = 0
	msgFramebufferUpdateReq = 3
)

func serveWebsocket(c *websocket.Conn, f *fb, serverName string) {
	defer c.Close()

	clientFB := f
	log.Printf("[%s] Client connected from %s", serverName, c.RemoteAddr())

	// send server protocol version
	err := c.WriteMessage(websocket.BinaryMessage, []byte(rfbVersion))
	if err != nil {
		log.Printf("[%s] Failed to send version: %v", serverName, err)
		return
	}

	// read client protocol version
	_, messageBody, err := c.ReadMessage()
	if err != nil {
		log.Printf("[%s] Failed to read client version: %v", serverName, err)
		return
	}
	if len(messageBody) != 12 {
		log.Printf("[%s] Failed to read client version", serverName)
		return
	}

	// send auth type
	err = c.WriteMessage(websocket.BinaryMessage, []byte{1, 1})
	if err != nil {
		log.Printf("[%s] Failed to send auth: %v", serverName, err)
		return
	}

	// read client auth selection
	_, messageBody, err = c.ReadMessage()
	if err != nil {
		log.Printf("[%s] Failed to read auth selection: %v", serverName, err)
		return
	}
	if len(messageBody) != 1 {
		log.Printf("[%s] Failed to read auth selection", serverName)
		return
	}

	// send auth result
	err = c.WriteMessage(websocket.BinaryMessage, make([]byte, 4))
	if err != nil {
		log.Printf("[%s] Failed to send auth result: %v", serverName, err)
		return
	}

	// read client init
	_, messageBody, err = c.ReadMessage()
	if err != nil {
		log.Printf("[%s] Failed to read client init: %v", serverName, err)
		return
	}
	if len(messageBody) != 1 {
		log.Printf("[%s] Failed to read client init", serverName)
		return
	}

	// send server init
	pf := pixelFormat{32, 24, 0, 1, 255, 255, 255, 16, 8, 0, [3]byte{}}
	serverInitMessage, err := constructServerInit(
		clientFB.w,
		clientFB.h,
		pf,
		serverName,
	)
	if err != nil {
		log.Printf("[%s] Failed to construct server init: %v", serverName, err)
		return
	}
	err = c.WriteMessage(websocket.BinaryMessage, serverInitMessage)
	if err != nil {
		log.Printf("[%s] Failed to send server init: %v", serverName, err)
		return
	}

	var lastRejectedFormat string

	// mainloop: track encoding format and send framebuffer updates
	for {
		_, messageBody, err = c.ReadMessage()
		if err != nil {
			log.Printf("[%s] Failed to read client message: %v", serverName, err)
			return
		}
		if len(messageBody) == 0 {
			log.Printf("[%s] Failed to read client message", serverName)
			return
		}

		msgType := messageBody[0]
		switch msgType {
		case msgSetPixelFormat:
			if len(messageBody) == 20 {
				var want pixelFormat
				binary.Read(bytes.NewReader(messageBody[4:]), binary.BigEndian, &want)
				formatID := fmt.Sprintf("%dbpp trueColor=%d", want.BPP, want.TrueColor)
				if want.TrueColor == 1 && (want.BPP == 32 || want.BPP == 24 || want.BPP == 8) {
					pf = want
					log.Printf("[%s] Client pixel format: %s", serverName, formatID)
					lastRejectedFormat = ""
				} else {
					if formatID != lastRejectedFormat {
						log.Printf("[%s] Unsupported format: %s — ignoring", serverName, formatID)
						lastRejectedFormat = formatID
					}
				}

			}
		case msgFramebufferUpdateReq:
			framebufferMessage, err := constructFramebufferMessage(clientFB, pf)
			if err != nil {
				log.Printf("[%s] Failed to construct framebuffer message: %v", serverName, err)
				return
			}
			err = c.WriteMessage(websocket.BinaryMessage, framebufferMessage)
			if err != nil {
				log.Printf("[%s] Failed to send framebuffer: %v", serverName, err)
				return
			}
		}
	}
}

func constructServerInit(width, height int, pf pixelFormat, name string) ([]byte, error) {
	b := new(bytes.Buffer)

	err := write16(b, uint16(width), uint16(height))
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.BigEndian, pf)
	if err != nil {
		return nil, err
	}
	err = write32(b, uint32(len(name)))
	if err != nil {
		return nil, err
	}
	_, err = b.Write([]byte(name))
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

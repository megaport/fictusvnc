// vncpng_realvnc_fixed.go
// Minimal VNC server that sends a static PNG to any client in RAW encoding.
// Supports 8/24/32 bpp true-color pixel formats (including RealVNC low quality).
// Usage: go run vncpng_realvnc_fixed.go :5900 image.png

package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type pixelFormat struct {
	BPP, Depth             uint8
	BigEndian, TrueColor   uint8
	RMax, GMax, BMax       uint16
	RShift, GShift, BShift uint8
	_                      [3]byte // padding
}

type fb struct {
	w, h int
	data []byte
}

type Config struct {
	Server []ServerConfig `toml:"server"`
}

type ServerConfig struct {
	Listen string `toml:"listen"`
	Image  string `toml:"image"`
	Name   string `toml:"server_name"`
}

const (
	appVersion              = "1.0.0"
	rfbVersion              = "RFB 003.008\n"
	msgSetPixelFormat       = 0
	msgSetEncodings         = 2
	msgFramebufferUpdateReq = 3
	msgEnableCU             = 150
	defaultImageDir         = "images"
)

var defaultName string
var noBrand bool
var showVersion bool

func main() {
	configPath := flag.String("config", "", "Path to TOML config file (default: ./servers.toml)")
	defaultNameFlag := flag.String("name", "FictusVNC", "Default server name")
	noBrandFlag := flag.Bool("no-brand", false, "Disable 'FictusVNC - ' prefix in server name")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.BoolVar(&showVersion, "v", false, "Show version and exit (shorthand)")
	flag.Parse()

	defaultName = *defaultNameFlag
	noBrand = *noBrandFlag

	if showVersion {
		fmt.Printf("FictusVNC %s\n", appVersion)
		return
	}

	log.Printf("[INFO] FictusVNC %s starting…", appVersion)

	if *configPath == "" {
		exe, _ := os.Executable()
		dir := filepath.Dir(exe)
		*configPath = filepath.Join(dir, "servers.toml")
	}

	var cfg Config
	if _, err := os.Stat(*configPath); err == nil {
		_, err := toml.DecodeFile(*configPath, &cfg)
		check(err)
		for _, s := range cfg.Server {
			imagePath := filepath.Join(defaultImageDir, s.Image)
			img, err := loadPNG(imagePath)
			if err != nil {
				log.Printf("[ERROR] loading %s: %v", imagePath, err)
				continue
			}
			name := s.Name
			if name == "" {
				name = defaultName
			}
			if !noBrand {
				name = fmt.Sprintf("FictusVNC - %s", name)
			}
			go runVNCServer(s.Listen, img, name)
		}
		select {}
	} else if flag.NArg() == 2 {
		addr, path := flag.Arg(0), flag.Arg(1)
		if !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
		img, err := loadPNG(path)
		check(err)
		name := defaultName
		if !noBrand {
			name = fmt.Sprintf("FictusVNC - %s", name)
		}
		runVNCServer(addr, img, name)
		select {}
	} else {
		// fallback default config
		imagePath := filepath.Join(defaultImageDir, "default.png")
		img, err := loadPNG(imagePath)
		if err != nil {
			log.Fatalf("No config or arguments, and failed to load default image at %s: %v", imagePath, err)
		}
		name := defaultName
		if !noBrand {
			name = fmt.Sprintf("FictusVNC - %s", name)
		}
		addr := "127.0.0.1:5900"
		log.Printf("[INFO] No config or args, starting default server at %s", addr)
		go runVNCServer(addr, img, name)
		select {}
	}
}

func runVNCServer(addr string, f *fb, serverName string) {
	ln, err := net.Listen("tcp", addr)
	check(err)
	log.Printf("[%s] Serving PNG %dx%d on %s", serverName, f.w, f.h, addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("[%s] Accept error: %v", serverName, err)
			time.Sleep(time.Second)
			continue
		}
		go serve(c, f, serverName)
	}
}

func serve(c net.Conn, f *fb, serverName string) {
	defer func() {
		c.Close()
		log.Printf("[%s] Client disconnected", serverName)
	}()

	log.Printf("[%s] Client connected from %s", serverName, c.RemoteAddr())

	// Add read deadline to prevent goroutine leaks
	c.SetReadDeadline(time.Now().Add(30 * time.Second))

	// VNC handshake
	_, err := c.Write([]byte(rfbVersion))
	if err != nil {
		log.Printf("[%s] Failed to send version: %v", serverName, err)
		return
	}

	buf := make([]byte, 12)
	_, err = io.ReadFull(c, buf)
	if err != nil {
		log.Printf("[%s] Failed to read client version: %v", serverName, err)
		return
	}

	_, err = c.Write([]byte{1, 1})
	if err != nil {
		log.Printf("[%s] Failed to send auth: %v", serverName, err)
		return
	}

	buf = make([]byte, 1)
	_, err = io.ReadFull(c, buf)
	if err != nil {
		log.Printf("[%s] Failed to read auth selection: %v", serverName, err)
		return
	}

	_, err = c.Write(make([]byte, 4))
	if err != nil {
		log.Printf("[%s] Failed to send auth result: %v", serverName, err)
		return
	}

	buf = make([]byte, 1)
	_, err = io.ReadFull(c, buf)
	if err != nil {
		log.Printf("[%s] Failed to read client init: %v", serverName, err)
		return
	}

	pf := pixelFormat{32, 24, 0, 1, 255, 255, 255, 16, 8, 0, [3]byte{}}
	err = sendServerInit(c, f.w, f.h, pf, serverName)
	if err != nil {
		log.Printf("[%s] Failed to send server init: %v", serverName, err)
		return
	}

	var lastRejectedFormat string

	for {
		// Reset read deadline for each message
		err := c.SetReadDeadline(time.Now().Add(30 * time.Second))
		if err != nil {
			log.Printf("[%s] Failed to set read deadline: %v", serverName, err)
			return
		}

		msgType, err := read1(c)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("[%s] Read timeout, closing connection", serverName)
			} else if err != io.EOF {
				log.Printf("[%s] Read error: %v", serverName, err)
			}
			return
		}

		switch msgType {
		case msgSetPixelFormat:
			_, err := readN(c, 3)
			if err != nil {
				log.Printf("[%s] Failed to read padding: %v", serverName, err)
				return
			}

			var buf [16]byte
			_, err = io.ReadFull(c, buf[:])
			if err != nil {
				log.Printf("[%s] Failed to read pixel format: %v", serverName, err)
				return
			}

			var want pixelFormat
			binary.Read(bytes.NewReader(buf[:]), binary.BigEndian, &want)
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
		case msgSetEncodings:
			_, err := readN(c, 1)
			if err != nil {
				return
			}

			n, err := read16(c)
			if err != nil {
				return
			}

			_, err = readN(c, int(n)*4)
			if err != nil {
				return
			}
		case msgEnableCU:
			_, err := readN(c, 9)
			if err != nil {
				return
			}
		case msgFramebufferUpdateReq:
			_, err := readN(c, 9)
			if err != nil {
				return
			}

			err = sendFramebuffer(c, f, pf)
			if err != nil {
				log.Printf("[%s] Failed to send framebuffer: %v", serverName, err)
				return
			}
		default:
			_, err := readN(c, 255)
			if err != nil {
				return
			}
		}
	}
}

func sendServerInit(c net.Conn, w, h int, pf pixelFormat, name string) error {
	err := write16(c, uint16(w), uint16(h))
	if err != nil {
		return err
	}

	err = binary.Write(c, binary.BigEndian, pf)
	if err != nil {
		return err
	}

	err = write32(c, uint32(len(name)))
	if err != nil {
		return err
	}

	_, err = c.Write([]byte(name))
	return err
}

func sendFramebuffer(c net.Conn, f *fb, pf pixelFormat) error {
	_, err := c.Write([]byte{0, 0})
	if err != nil {
		return err
	}

	err = write16(c, 1)
	if err != nil {
		return err
	}

	err = write16(c, 0, 0, uint16(f.w), uint16(f.h))
	if err != nil {
		return err
	}

	err = write32(c, 0)
	if err != nil {
		return err
	}

	conv := converter(pf)
	bpp := int(pf.BPP / 8)
	line := make([]byte, f.w*bpp)

	for y := range f.h {
		i := 0
		for x := 0; x < f.w; x++ {
			off := y*f.w*4 + x*4
			r, g, b := f.data[off+2], f.data[off+1], f.data[off+0]
			pix := conv(r, g, b)
			copy(line[i:], pix)
			i += len(pix)
		}
		_, err := c.Write(line[:i])
		if err != nil {
			return err
		}
	}

	return nil
}

func converter(pf pixelFormat) func(r, g, b uint8) []byte {
	switch pf.BPP {
	case 32:
		sh := []uint8{pf.RShift, pf.GShift, pf.BShift}
		return func(r, g, b uint8) []byte {
			c := []uint32{uint32(r), uint32(g), uint32(b)}
			out := uint32(0)
			for i := 0; i < 3; i++ {
				out |= (c[i] & uint32(pf.RMax)) << sh[i]
			}
			buf := make([]byte, 4)
			if pf.BigEndian != 0 {
				binary.BigEndian.PutUint32(buf, out)
			} else {
				binary.LittleEndian.PutUint32(buf, out)
			}
			return buf
		}
	case 24:
		return func(r, g, b uint8) []byte {
			return []byte{r, g, b}
		}
	case 8:
		if pf.RMax == 7 && pf.GMax == 7 && pf.BMax == 3 && pf.RShift == 5 && pf.GShift == 2 && pf.BShift == 0 {
			return func(r, g, b uint8) []byte {
				rr := r >> 5
				gg := g >> 5
				bb := b >> 6
				return []byte{(rr << 5) | (gg << 2) | bb}
			}
		}
	}
	return func(r, g, b uint8) []byte {
		y := (uint32(r)*30 + uint32(g)*59 + uint32(b)*11 + 50) / 100
		return []byte{uint8(y)}
	}
}

func loadPNG(path string) (*fb, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	r := img.Bounds()
	buf := make([]byte, r.Dx()*r.Dy()*4)
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			off := y*r.Dx()*4 + x*4
			rr, gg, bb, _ := img.At(x+r.Min.X, y+r.Min.Y).RGBA()
			buf[off+0] = byte(bb >> 8)
			buf[off+1] = byte(gg >> 8)
			buf[off+2] = byte(rr >> 8)
			buf[off+3] = 0
		}
	}
	return &fb{r.Dx(), r.Dy(), buf}, nil
}

func write16(c net.Conn, v ...uint16) error {
	for _, x := range v {
		err := binary.Write(c, binary.BigEndian, x)
		if err != nil {
			return err
		}
	}
	return nil
}

func write32(c net.Conn, v uint32) error {
	return binary.Write(c, binary.BigEndian, v)
}

func readN(c net.Conn, n int) (int64, error) {
	return io.CopyN(io.Discard, c, int64(n))
}

func read1(c net.Conn) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(c, b[:])
	return b[0], err
}

func read16(c net.Conn) (uint16, error) {
	var v uint16
	err := binary.Read(c, binary.BigEndian, &v)
	return v, err
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

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

func main() {
	configPath := flag.String("config", "", "Path to TOML config file (default: ./servers.toml)")
	defaultNameFlag := flag.String("name", "FictusVNC", "Default server name")
	noBrandFlag := flag.Bool("no-brand", false, "Disable 'FictusVNC - ' prefix in server name")
	flag.Parse()

	defaultName = *defaultNameFlag
	noBrand = *noBrandFlag

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
			continue
		}
		go serve(c, f, serverName)
	}
}

func serve(c net.Conn, f *fb, serverName string) {
	defer c.Close()
	write(c, []byte(rfbVersion))
	readN(c, 12)
	write(c, []byte{1, 1})
	readN(c, 1)
	write(c, make([]byte, 4))
	readN(c, 1)

	pf := pixelFormat{32, 24, 0, 1, 255, 255, 255, 16, 8, 0, [3]byte{}}
	sendServerInit(c, f.w, f.h, pf, serverName)

	var lastRejectedFormat string

	for {
		switch read1(c) {
		case msgSetPixelFormat:
			readN(c, 3)
			var buf [16]byte
			readFull(c, buf[:])
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
			readN(c, 1)
			n := read16(c)
			readN(c, int(n)*4)
		case msgEnableCU:
			readN(c, 9)
		case msgFramebufferUpdateReq:
			readN(c, 9)
			sendFramebuffer(c, f, pf)
		default:
			readN(c, 255)
		}
	}
}

func sendServerInit(c net.Conn, w, h int, pf pixelFormat, name string) {
	write16(c, uint16(w), uint16(h))
	binary.Write(c, binary.BigEndian, pf)
	write32(c, uint32(len(name)))
	write(c, []byte(name))
}

func sendFramebuffer(c net.Conn, f *fb, pf pixelFormat) {
	write(c, []byte{0, 0})
	write16(c, 1)
	write16(c, 0, 0, uint16(f.w), uint16(f.h))
	write32(c, 0)

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
		write(c, line[:i])
	}
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

func write(c net.Conn, b []byte) { _, _ = c.Write(b) }
func write16(c net.Conn, v ...uint16) {
	for _, x := range v {
		binary.Write(c, binary.BigEndian, x)
	}
}
func write32(c net.Conn, v uint32)   { binary.Write(c, binary.BigEndian, v) }
func readN(c net.Conn, n int)        { io.CopyN(io.Discard, c, int64(n)) }
func read1(c net.Conn) byte          { var b [1]byte; io.ReadFull(c, b[:]); return b[0] }
func read16(c net.Conn) uint16       { var v uint16; binary.Read(c, binary.BigEndian, &v); return v }
func readFull(r io.Reader, b []byte) { _, _ = io.ReadFull(r, b) }
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

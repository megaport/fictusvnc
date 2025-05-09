// main.go
// Minimal VNC server main entry point and basic utilities.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Importing global variables from config.go
var (
	defaultName string
	noBrand     bool
	showVersion bool
	showIP      bool
)

func main() {
	configPath := flag.String("config", "", "Path to TOML config file (default: ./servers.toml)")
	defaultNameFlag := flag.String("name", "FictusVNC", "Default server name")
	noBrandFlag := flag.Bool("no-brand", false, "Disable 'FictusVNC - ' prefix in server name")
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.BoolVar(&showVersion, "v", false, "Show version and exit (shorthand)")
	flag.BoolVar(&showIP, "show-ip", false, "Show client IP on image")
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

			// Handle port range if specified
			if s.StartPort > 0 && s.EndPort > 0 && s.EndPort >= s.StartPort {
				for port := s.StartPort; port <= s.EndPort; port++ {
					addr := fmt.Sprintf(":%d", port)
					if s.Listen != "" && !strings.HasPrefix(s.Listen, ":") {
						host := strings.Split(s.Listen, ":")[0]
						addr = fmt.Sprintf("%s:%d", host, port)
					}
					serverName := fmt.Sprintf("%s (Port %d)", name, port)

					// Start server in a separate goroutine and handle errors
					go func(addr, serverName string) {
						if err := runVNCServer(addr, img, serverName, showIP); err != nil {
							log.Printf("[WARN] Failed to start server %s on %s: %v", serverName, addr, err)
						}
					}(addr, serverName)
				}
			} else if s.Listen != "" {
				go func() {
					if err := runVNCServer(s.Listen, img, name, showIP); err != nil {
						log.Printf("[WARN] Failed to start server %s on %s: %v", name, s.Listen, err)
					}
				}()
			} else {
				log.Printf("[ERROR] Server %s has no listen address or valid port range", name)
			}
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
		if err := runVNCServer(addr, img, name, showIP); err != nil {
			log.Printf("[ERROR] Failed to start server: %v", err)
			os.Exit(1)
		}
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
		go func() {
			if err := runVNCServer(addr, img, name, showIP); err != nil {
				log.Printf("[ERROR] Failed to start default server: %v", err)
				os.Exit(1)
			}
		}()
		select {}
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

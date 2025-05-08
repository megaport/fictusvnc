package main

const (
	appVersion              = "1.0.0"
	rfbVersion              = "RFB 003.008\n"
	msgSetPixelFormat       = 0
	msgSetEncodings         = 2
	msgFramebufferUpdateReq = 3
	msgEnableCU             = 150
	defaultImageDir         = "images"
)

type Config struct {
	Server []ServerConfig `toml:"server"`
}

type ServerConfig struct {
	Listen string `toml:"listen"`
	Image  string `toml:"image"`
	Name   string `toml:"server_name"`
}

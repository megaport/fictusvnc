#!/bin/bash
set -e

GREEN="\e[32m"
RED="\e[31m"
RESET="\e[0m"

MODE="release"
if [[ "$1" == "debug" ]]; then
  MODE="debug"
fi

echo -e "${GREEN}📦 Checking Go installation...${RESET}"
if ! command -v go &> /dev/null; then
  echo -e "${RED}Go is not installed. Please install Go first: https://go.dev/dl/${RESET}"
  exit 1
fi

echo -e "${GREEN}⬇️  Downloading dependencies...${RESET}"
go mod tidy

echo -e "${GREEN}🔨 Building in '$MODE' mode...${RESET}"
FLAGS=""
if [[ "$MODE" == "release" ]]; then
  FLAGS=(-ldflags=-s -w)
fi

go build $FLAGS -o fictusvnc main.go

echo -e "${GREEN}✅ Build complete: ./fictusvnc${RESET}"
echo ""
echo "Run the server with:"
echo "  ./fictusvnc"
echo ""
echo "Install globally with:"
echo "  sudo mv fictusvnc /usr/local/bin/"
echo "  sudo chmod +x /usr/local/bin/fictusvnc"

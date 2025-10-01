# FictusVNC Server

A minimal VNC server that serves a static image.

![FictusVNC](banner.png)

---

## ⚙️ Features

- 🖼 Serve static JPG & PNG as framebuffer
- 🖥 Supports websocket-based clients clients (noVNC)
- 🔓 SSL connection
- 💾 Cross-platform: Linux, Windows, macOS, ARM64
- 📉 Lightweight: ~9.2MB binary

---

## 🚀 Quick Start

- [▶️ Run from command line](#run-from-command-line)
- [🗂 Preview](#preview)

---

### ▶️ Run from command line

```bash
./fictusvnc-linux-amd64 -certfile cert.pem -keyfile key.pem  -image ./images/default.png -port 5900 -servername "Test server"
```

---

### 🗂 Preview

![FictusVNC](vncwindow.png)

---

## Available Flags

| Flag              | Description       | Default Value          |
| ----------------- | ----------------- | ---------------------- |
| `-port`           | Port to listen on | 5900                   |
| `-certfile`       | Certificate file  | `./cert.pem`           |
| `-keyfile`        | Key file          | `./key.pem`            |
| `-image`          | Image to display  | `./images/default.png` |
| `-servername`     | VNC server name   | `Mock VNC server`      |

---

## License

This project is licensed under the terms specified in the [LICENSE](LICENSE) file.


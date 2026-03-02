# Tutorial Deploy Traefik Config API Generator

## 📋 Prerequisites

- Server/VM dengan Ubuntu 20.04+ atau Debian 11+
- Go 1.21+ terinstall
- Traefik sudah terinstall dan berjalan
- Root atau sudo access

---

## 🚀 Method 1: Deploy Manual (Development)

### 1. Setup Project

```bash
# Login ke server
ssh user@your-server

# Buat direktori project
mkdir -p ~/traefik-config-api
cd ~/traefik-config-api

# Buat file main.go
nano main.go
# Copy paste code dari artifact

# Initialize Go module
go mod init traefik-config-api

# Install dependencies
go get github.com/gorilla/mux
go get gopkg.in/yaml.v3

# Download dependencies
go mod tidy
```

### 2. Buat Direktori Data

```bash
# Buat direktori untuk menyimpan config
mkdir -p /etc/traefik/dynamic
mkdir -p ~/traefik-config-api/data

# Set permissions
chmod 755 /etc/traefik/dynamic
```

### 3. Test Run

```bash
# Test run aplikasi
DATA_DIR=/etc/traefik/dynamic OUTPUT_FORMAT=yaml PORT=8080 go run main.go
```

### 4. Build Binary

```bash
# Build aplikasi
go build -o traefik-config-api main.go

# Test binary
./traefik-config-api
```

---

## 🐳 Method 2: Deploy dengan Docker

### 1. Buat Dockerfile

```bash
nano Dockerfile
```

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o traefik-config-api .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/traefik-config-api .

# Create data directory
RUN mkdir -p /data

# Expose port
EXPOSE 8080

# Set environment variables
ENV DATA_DIR=/data
ENV OUTPUT_FORMAT=yaml
ENV PORT=8080

CMD ["./traefik-config-api"]
```

### 2. Buat docker-compose.yml

```bash
nano docker-compose.yml
```

```yaml
version: '3.8'

services:
  traefik-config-api:
    build: .
    container_name: traefik-config-api
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
      - /etc/traefik/dynamic:/etc/traefik/dynamic
    environment:
      - DATA_DIR=/data
      - OUTPUT_FORMAT=yaml
      - PORT=8080
    networks:
      - traefik-network

networks:
  traefik-network:
    external: true
```

### 3. Build dan Run

```bash
# Build image
docker-compose build

# Start container
docker-compose up -d

# Check logs
docker-compose logs -f
```

---

## 🔧 Method 3: Deploy dengan Systemd (Production)

### 1. Setup Binary

```bash
# Build binary
cd ~/traefik-config-api
go build -o traefik-config-api main.go

# Copy ke /usr/local/bin
sudo cp traefik-config-api /usr/local/bin/
sudo chmod +x /usr/local/bin/traefik-config-api

# Buat user untuk service
sudo useradd -r -s /bin/false traefik-api
```

### 2. Buat Direktori Data

```bash
# Buat direktori
sudo mkdir -p /var/lib/traefik-api
sudo mkdir -p /etc/traefik/dynamic

# Set ownership
sudo chown -R traefik-api:traefik-api /var/lib/traefik-api
sudo chown -R traefik-api:traefik-api /etc/traefik/dynamic
```

### 3. Buat Systemd Service

```bash
sudo nano /etc/systemd/system/traefik-config-api.service
```

```ini
[Unit]
Description=Traefik Config API Generator
After=network.target

[Service]
Type=simple
User=traefik-api
Group=traefik-api
ExecStart=/usr/local/bin/traefik-config-api
Restart=on-failure
RestartSec=5s

# Environment variables
Environment="DATA_DIR=/var/lib/traefik-api"
Environment="OUTPUT_FORMAT=yaml"
Environment="PORT=8080"

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/traefik-api /etc/traefik/dynamic

[Install]
WantedBy=multi-user.target
```

### 4. Start Service

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable traefik-config-api

# Start service
sudo systemctl start traefik-config-api

# Check status
sudo systemctl status traefik-config-api

# View logs
sudo journalctl -u traefik-config-api -f
```

---

## ⚙️ Konfigurasi Traefik

### 1. Update traefik.yml

```bash
sudo nano /etc/traefik/traefik.yml
```

Tambahkan konfigurasi file provider:

```yaml
# File Provider untuk dynamic config
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true

# Entry points
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

# API dan Dashboard (optional)
api:
  dashboard: true
  insecure: false

# Logging
log:
  level: INFO

# Access Log
accessLog: {}
```

### 2. Restart Traefik

```bash
# Jika menggunakan systemd
sudo systemctl restart traefik

# Jika menggunakan docker
docker restart traefik
```

---

## 🧪 Testing API

### 1. Health Check

```bash
curl http://localhost:8080/health
```

### 2. Create Route

```bash
curl -X POST http://localhost:8080/api/v1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-app",
    "domain": "test.example.com",
    "service_url": "http://192.168.1.100:3000",
    "tls": true,
    "middlewares": ["default-headers"],
    "pass_host_header": true
  }'
```

### 3. Get All Routes

```bash
curl http://localhost:8080/api/v1/routes
```

### 4. Get Single Route

```bash
curl http://localhost:8080/api/v1/routes/test-app
```

### 5. Update Route

```bash
curl -X PUT http://localhost:8080/api/v1/routes/test-app \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "test.example.com",
    "service_url": "http://192.168.1.101:3001",
    "tls": true,
    "middlewares": ["default-headers"]
  }'
```

### 6. Delete Route

```bash
curl -X DELETE http://localhost:8080/api/v1/routes/test-app
```

### 7. Verify Generated Config

```bash
# Check YAML file
cat /etc/traefik/dynamic/traefik-dynamic.yaml

# Check Traefik logs
sudo journalctl -u traefik -f
```

---

## 🔒 Securing the API

### 1. Dengan Nginx Reverse Proxy

```bash
sudo apt install nginx
sudo nano /etc/nginx/sites-available/traefik-api
```

```nginx
server {
    listen 80;
    server_name api.your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # Basic Auth
        auth_basic "Restricted Access";
        auth_basic_user_file /etc/nginx/.htpasswd;
    }
}
```

```bash
# Create htpasswd
sudo apt install apache2-utils
sudo htpasswd -c /etc/nginx/.htpasswd admin

# Enable site
sudo ln -s /etc/nginx/sites-available/traefik-api /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 2. Dengan Traefik Basic Auth

Tambahkan di API route Anda:

```bash
curl -X POST http://localhost:8080/api/v1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "id": "traefik-config-api",
    "domain": "api.your-domain.com",
    "service_url": "http://localhost:8080",
    "tls": true,
    "middlewares": ["default-headers", "api-auth"]
  }'
```

Buat middleware auth di Traefik:

```yaml
# /etc/traefik/dynamic/middlewares.yaml
http:
  middlewares:
    api-auth:
      basicAuth:
        users:
          - "admin:$apr1$xxx..." # generated with htpasswd
```

---

## 🔧 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Port untuk API server |
| `DATA_DIR` | `./data` | Direktori untuk menyimpan routes.json |
| `OUTPUT_FORMAT` | `yaml` | Format output (yaml/json) |

---

## 📝 File Structure

```
/usr/local/bin/traefik-config-api          # Binary
/var/lib/traefik-api/routes.json           # Routes database
/etc/traefik/dynamic/traefik-dynamic.yaml  # Generated config
/etc/systemd/system/traefik-config-api.service  # Service file
```

---

## 🐛 Troubleshooting

### API tidak bisa diakses

```bash
# Check service status
sudo systemctl status traefik-config-api

# Check logs
sudo journalctl -u traefik-config-api -n 50

# Check port
sudo netstat -tlnp | grep 8080
```

### Traefik tidak reload config

```bash
# Check Traefik logs
sudo journalctl -u traefik -f

# Verify file permissions
ls -la /etc/traefik/dynamic/

# Manual restart Traefik
sudo systemctl restart traefik
```

### Permission denied

```bash
# Fix ownership
sudo chown -R traefik-api:traefik-api /var/lib/traefik-api
sudo chown -R traefik-api:traefik-api /etc/traefik/dynamic

# Fix permissions
sudo chmod 755 /etc/traefik/dynamic
sudo chmod 644 /etc/traefik/dynamic/*.yaml
```

---

## 🔄 Update/Upgrade

```bash
# Pull latest code
cd ~/traefik-config-api
git pull  # jika menggunakan git

# Rebuild
go build -o traefik-config-api main.go

# Copy binary
sudo cp traefik-config-api /usr/local/bin/

# Restart service
sudo systemctl restart traefik-config-api
```

---

## 📊 Monitoring

### Check Logs

```bash
# Real-time logs
sudo journalctl -u traefik-config-api -f

# Last 100 lines
sudo journalctl -u traefik-config-api -n 100

# Filter by date
sudo journalctl -u traefik-config-api --since "2024-01-01" --until "2024-01-31"
```

### Check Traefik Dashboard

Access Traefik dashboard untuk melihat routes yang sudah terdaftar:
- URL: `http://your-server:8080/dashboard/`

---

## 🎯 Best Practices

1. **Backup routes.json** secara berkala
2. **Gunakan HTTPS** untuk production
3. **Enable authentication** untuk API
4. **Monitor logs** untuk error
5. **Set resource limits** di systemd service
6. **Use version control** untuk track changes
7. **Test** di staging environment dulu

---

## 📚 Example Use Cases

### 1. Development Environment

```bash
# Developer deploy service baru
curl -X POST http://api.internal/api/v1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-feature-branch",
    "domain": "my-feature.dev.company.com",
    "service_url": "http://192.168.1.50:8080",
    "tls": true
  }'
```

### 2. Multiple Services

```bash
# Proxmox
curl -X POST http://localhost:8080/api/v1/routes \
  -d '{"id":"proxmox","domain":"prx.example.com","service_url":"https://172.16.1.6:8006","tls":true,"middlewares":["default-headers"],"pass_host_header":true}'

# Portainer
curl -X POST http://localhost:8080/api/v1/routes \
  -d '{"id":"portainer","domain":"portainer.example.com","service_url":"https://172.16.1.7:9443","tls":true,"pass_host_header":true}'

# GitLab
curl -X POST http://localhost:8080/api/v1/routes \
  -d '{"id":"gitlab","domain":"git.example.com","service_url":"http://172.16.1.8:80","tls":true}'
```

---

## 🎉 Selesai!

API sudah berjalan dan siap digunakan. Setiap kali developer perlu domain baru, tinggal hit API dan Traefik akan otomatis reload config!



Saya sudah buatkan tutorial lengkap deployment! Ada 3 metode yang bisa dipilih:
📌 Ringkasan Metode:

Manual/Development - Cocok untuk testing dan development
Docker - Paling mudah dan portable
Systemd - Production-ready dengan auto-restart

🚀 Quick Start (Paling Cepat):
Untuk testing cepat:

```bash
# Clone/setup
mkdir traefik-config-api && cd traefik-config-api
# Copy code ke main.go
go mod init traefik-config-api
go get github.com/gorilla/mux gopkg.in/yaml.v3
go run main.go
```

Untuk production:

```bash
# Build
go build -o traefik-config-api main.go
sudo cp traefik-config-api /usr/local/bin/

# Setup systemd (copy service file dari tutorial)
sudo systemctl enable traefik-config-api
sudo systemctl start traefik-config-api
```


🔥 Yang Paling Penting:

Traefik harus dikonfigurasi untuk watch directory /etc/traefik/dynamic
API akan generate file traefik-dynamic.yaml di directory tersebut
Traefik auto-reload setiap ada perubahan file
Protect API dengan basic auth atau reverse proxy

💡 Tips:

Gunakan Docker jika ingin mudah dan portable
Gunakan Systemd untuk production yang stable
Jangan lupa backup file routes.json
Enable authentication sebelum expose ke internet

Apakah ada yang ingin ditanyakan atau butuh bantuan dengan step tertentu?

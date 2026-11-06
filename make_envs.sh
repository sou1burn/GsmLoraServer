#!/bin/bash

cat > db.env << EOF
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secure_password_123
POSTGRES_DB=lora_db
PORT=9090
DB_PORT=5433
DB_USER= postgres
DB_PASSWORD= secret
DB_NAME= lora_db
DB_HOST=localhost
ENV=dev
EOF

cat > esp.env << EOF
ESP_HOST=esp32-6FAD74
ESP_PORT=9090
ESP_MAC=7C:9E:BD:6F:AD:74
ESP_IP=
EOF

cat > client.env << EOF
CLIENT_IP=127.0.0.1
CLIENT_PORT=9090
EOF

echo "Созданы файлы: .env.example, .env.local, .env.production"
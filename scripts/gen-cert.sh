#!/usr/bin/env sh
set -eu

OUT_DIR="${1:-./certs}"
HOSTS="${2:-localhost,127.0.0.1}"
DAYS="${3:-365}"

mkdir -p "$OUT_DIR"
KEY_FILE="$OUT_DIR/sharehere.key"
CERT_FILE="$OUT_DIR/sharehere.crt"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required" >&2
  exit 1
fi

# Generates a local self-signed cert for LAN/dev usage.
openssl req -x509 -newkey rsa:4096 -sha256 -days "$DAYS" -nodes \
  -keyout "$KEY_FILE" \
  -out "$CERT_FILE" \
  -subj "/CN=sharehere" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

echo "Generated:"
echo "  $CERT_FILE"
echo "  $KEY_FILE"
echo "Run: sharehere --https --cert $CERT_FILE --key $KEY_FILE"
echo "For trusted local certs, prefer mkcert: https://github.com/FiloSottile/mkcert"

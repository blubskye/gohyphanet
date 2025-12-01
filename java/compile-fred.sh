#!/bin/bash
# Compile essential Fred classes for handshake

set -e

FRED_SRC="/home/blubskye/Downloads/fred-next/src"
BUILD_DIR="/home/blubskye/Downloads/gohyphanet/java/build/fred"
CLASSES_DIR="/home/blubskye/Downloads/gohyphanet/java/build/classes"

echo "Creating build directory..."
mkdir -p "$BUILD_DIR"

echo "Compiling Fred crypto classes..."

# Try to compile the essential classes
javac -d "$BUILD_DIR" \
    -cp "$BUILD_DIR" \
    "$FRED_SRC/freenet/crypt/SHA256.java" \
    "$FRED_SRC/freenet/crypt/HMAC.java" \
    "$FRED_SRC/freenet/crypt/Util.java" \
    "$FRED_SRC/freenet/crypt/BlockCipher.java" \
    "$FRED_SRC/freenet/crypt/CryptoKey.java" \
    "$FRED_SRC/freenet/crypt/CryptoElement.java" \
    "$FRED_SRC/freenet/crypt/UnsupportedCipherException.java" \
    2>&1 | head -50 || true

echo "Done"
ls -lh "$BUILD_DIR"

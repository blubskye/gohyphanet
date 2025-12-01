// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package freemail

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
)

// KeySize constants
const (
	AESKeySize   = 32 // AES-256
	AESBlockSize = 16
	RSAKeySize   = 2048
)

// GenerateRSAKeyPair generates a new RSA key pair
func GenerateRSAKeyPair() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, RSAKeySize)
}

// GenerateAESKey generates a random AES-256 key
func GenerateAESKey() ([]byte, error) {
	key := make([]byte, AESKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}
	return key, nil
}

// GenerateIV generates a random initialization vector
func GenerateIV() ([]byte, error) {
	iv := make([]byte, AESBlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}
	return iv, nil
}

// EncryptAES encrypts data using AES-256-CBC
func EncryptAES(plaintext, key, iv []byte) ([]byte, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("invalid key size: expected %d, got %d", AESKeySize, len(key))
	}
	if len(iv) != AESBlockSize {
		return nil, fmt.Errorf("invalid IV size: expected %d, got %d", AESBlockSize, len(iv))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// PKCS7 padding
	padding := AESBlockSize - (len(plaintext) % AESBlockSize)
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return ciphertext, nil
}

// DecryptAES decrypts data using AES-256-CBC
func DecryptAES(ciphertext, key, iv []byte) ([]byte, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("invalid key size: expected %d, got %d", AESKeySize, len(key))
	}
	if len(iv) != AESBlockSize {
		return nil, fmt.Errorf("invalid IV size: expected %d, got %d", AESBlockSize, len(iv))
	}
	if len(ciphertext)%AESBlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of block size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding > AESBlockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return plaintext[:len(plaintext)-padding], nil
}

// EncryptRSA encrypts data using RSA-OAEP with SHA-256
func EncryptRSA(plaintext []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, nil)
}

// DecryptRSA decrypts data using RSA-OAEP with SHA-256
func DecryptRSA(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, nil)
}

// SignSHA256 creates a SHA-256 signature using RSA-PSS
func SignSHA256(data []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.Sum256(data)
	return rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], nil)
}

// VerifySHA256 verifies a SHA-256 signature using RSA-PSS
func VerifySHA256(data, signature []byte, publicKey *rsa.PublicKey) error {
	hash := sha256.Sum256(data)
	return rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signature, nil)
}

// HashSHA256 computes SHA-256 hash
func HashSHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// EncodePublicKey encodes an RSA public key to PEM format
func EncodePublicKey(publicKey *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}

	return string(pem.EncodeToMemory(block)), nil
}

// DecodePublicKey decodes an RSA public key from PEM format
func DecodePublicKey(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPub, nil
}

// EncodePrivateKey encodes an RSA private key to PEM format
func EncodePrivateKey(privateKey *rsa.PrivateKey) string {
	der := x509.MarshalPKCS1PrivateKey(privateKey)

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	}

	return string(pem.EncodeToMemory(block))
}

// DecodePrivateKey decodes an RSA private key from PEM format
func DecodePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// EncodeBase64 encodes bytes to base64 string
func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes base64 string to bytes
func DecodeBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

// MessageCrypto provides encryption/decryption for Freemail messages
type MessageCrypto struct {
	privateKey *rsa.PrivateKey
}

// NewMessageCrypto creates a new MessageCrypto instance
func NewMessageCrypto(privateKey *rsa.PrivateKey) *MessageCrypto {
	return &MessageCrypto{
		privateKey: privateKey,
	}
}

// EncryptMessage encrypts a message for a recipient
func (mc *MessageCrypto) EncryptMessage(message []byte, recipientPublicKey *rsa.PublicKey) (*EncryptedMessage, error) {
	// Generate random AES key and IV
	aesKey, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	iv, err := GenerateIV()
	if err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt message with AES
	encryptedBody, err := EncryptAES(message, aesKey, iv)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	// Encrypt AES key with recipient's public key
	encryptedKey, err := EncryptRSA(aesKey, recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	// Sign the encrypted body
	signature, err := SignSHA256(encryptedBody, mc.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return &EncryptedMessage{
		EncryptedKey:  encryptedKey,
		IV:            iv,
		EncryptedBody: encryptedBody,
		Signature:     signature,
	}, nil
}

// DecryptMessage decrypts a message from a sender
func (mc *MessageCrypto) DecryptMessage(encrypted *EncryptedMessage, senderPublicKey *rsa.PublicKey) ([]byte, error) {
	// Verify signature if public key provided
	if senderPublicKey != nil {
		if err := VerifySHA256(encrypted.EncryptedBody, encrypted.Signature, senderPublicKey); err != nil {
			return nil, fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// Decrypt AES key with our private key
	aesKey, err := DecryptRSA(encrypted.EncryptedKey, mc.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// Decrypt message with AES
	plaintext, err := DecryptAES(encrypted.EncryptedBody, aesKey, encrypted.IV)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt message: %w", err)
	}

	return plaintext, nil
}

// EncryptedMessage represents an encrypted Freemail message
type EncryptedMessage struct {
	EncryptedKey  []byte // RSA-encrypted AES key
	IV            []byte // AES initialization vector
	EncryptedBody []byte // AES-encrypted message body
	Signature     []byte // RSA-PSS signature of encrypted body
}

// Serialize converts EncryptedMessage to bytes for transmission
func (em *EncryptedMessage) Serialize() []byte {
	// Format: keyLen(4) | key | ivLen(4) | iv | bodyLen(4) | body | sigLen(4) | sig
	keyLen := len(em.EncryptedKey)
	ivLen := len(em.IV)
	bodyLen := len(em.EncryptedBody)
	sigLen := len(em.Signature)

	totalLen := 16 + keyLen + ivLen + bodyLen + sigLen
	data := make([]byte, totalLen)

	offset := 0

	// Key length and key
	data[offset] = byte(keyLen >> 24)
	data[offset+1] = byte(keyLen >> 16)
	data[offset+2] = byte(keyLen >> 8)
	data[offset+3] = byte(keyLen)
	offset += 4
	copy(data[offset:], em.EncryptedKey)
	offset += keyLen

	// IV length and IV
	data[offset] = byte(ivLen >> 24)
	data[offset+1] = byte(ivLen >> 16)
	data[offset+2] = byte(ivLen >> 8)
	data[offset+3] = byte(ivLen)
	offset += 4
	copy(data[offset:], em.IV)
	offset += ivLen

	// Body length and body
	data[offset] = byte(bodyLen >> 24)
	data[offset+1] = byte(bodyLen >> 16)
	data[offset+2] = byte(bodyLen >> 8)
	data[offset+3] = byte(bodyLen)
	offset += 4
	copy(data[offset:], em.EncryptedBody)
	offset += bodyLen

	// Signature length and signature
	data[offset] = byte(sigLen >> 24)
	data[offset+1] = byte(sigLen >> 16)
	data[offset+2] = byte(sigLen >> 8)
	data[offset+3] = byte(sigLen)
	offset += 4
	copy(data[offset:], em.Signature)

	return data
}

// DeserializeEncryptedMessage parses bytes into an EncryptedMessage
func DeserializeEncryptedMessage(data []byte) (*EncryptedMessage, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("data too short")
	}

	offset := 0

	// Key length and key
	keyLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+keyLen > len(data) {
		return nil, fmt.Errorf("invalid key length")
	}
	encryptedKey := make([]byte, keyLen)
	copy(encryptedKey, data[offset:offset+keyLen])
	offset += keyLen

	// IV length and IV
	if offset+4 > len(data) {
		return nil, fmt.Errorf("data too short for IV length")
	}
	ivLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+ivLen > len(data) {
		return nil, fmt.Errorf("invalid IV length")
	}
	iv := make([]byte, ivLen)
	copy(iv, data[offset:offset+ivLen])
	offset += ivLen

	// Body length and body
	if offset+4 > len(data) {
		return nil, fmt.Errorf("data too short for body length")
	}
	bodyLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+bodyLen > len(data) {
		return nil, fmt.Errorf("invalid body length")
	}
	encryptedBody := make([]byte, bodyLen)
	copy(encryptedBody, data[offset:offset+bodyLen])
	offset += bodyLen

	// Signature length and signature
	if offset+4 > len(data) {
		return nil, fmt.Errorf("data too short for signature length")
	}
	sigLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+sigLen > len(data) {
		return nil, fmt.Errorf("invalid signature length")
	}
	signature := make([]byte, sigLen)
	copy(signature, data[offset:offset+sigLen])

	return &EncryptedMessage{
		EncryptedKey:  encryptedKey,
		IV:            iv,
		EncryptedBody: encryptedBody,
		Signature:     signature,
	}, nil
}

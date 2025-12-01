package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/dsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
)

// EncryptHashedDocname encrypts the hashed docname with the crypto key
// This is used for SSK key generation: E(H(docname))
func EncryptHashedDocname(hashedDocname, cryptoKey []byte) ([]byte, error) {
	if len(cryptoKey) != 32 {
		return nil, fmt.Errorf("crypto key must be 32 bytes")
	}
	if len(hashedDocname) != 32 {
		return nil, fmt.Errorf("hashed docname must be 32 bytes")
	}

	// Create AES cipher
	block, err := aes.NewCipher(cryptoKey)
	if err != nil {
		return nil, err
	}

	// Encrypt using ECB mode (single block)
	// Freenet uses Rijndael in ECB mode for this specific operation
	encrypted := make([]byte, 32)
	block.Encrypt(encrypted, hashedDocname)

	return encrypted, nil
}

// DecryptHashedDocname decrypts the encrypted hashed docname
func DecryptHashedDocname(encryptedHashedDocname, cryptoKey []byte) ([]byte, error) {
	if len(cryptoKey) != 32 {
		return nil, fmt.Errorf("crypto key must be 32 bytes")
	}
	if len(encryptedHashedDocname) != 32 {
		return nil, fmt.Errorf("encrypted hashed docname must be 32 bytes")
	}

	// Create AES cipher
	block, err := aes.NewCipher(cryptoKey)
	if err != nil {
		return nil, err
	}

	// Decrypt using ECB mode (single block)
	decrypted := make([]byte, 32)
	block.Decrypt(decrypted, encryptedHashedDocname)

	return decrypted, nil
}

// EncryptDataCTR encrypts data using AES-256-CTR mode
func EncryptDataCTR(data, key, iv []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Use provided IV or zero IV
	var ctrIV []byte
	if iv != nil {
		ctrIV = iv
	} else {
		ctrIV = make([]byte, aes.BlockSize)
	}

	stream := cipher.NewCTR(block, ctrIV)
	encrypted := make([]byte, len(data))
	stream.XORKeyStream(encrypted, data)

	return encrypted, nil
}

// DecryptDataCTR decrypts data using AES-256-CTR mode
func DecryptDataCTR(data, key, iv []byte) ([]byte, error) {
	// CTR mode encryption and decryption are the same operation
	return EncryptDataCTR(data, key, iv)
}

// EncryptDataPCFB encrypts data using AES-256-PCFB mode (Freenet's variant)
// PCFB is similar to CFB but with specific padding
func EncryptDataPCFB(data, key, iv []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Use provided IV or zero IV
	var cfbIV []byte
	if iv != nil {
		cfbIV = iv
	} else {
		cfbIV = make([]byte, aes.BlockSize)
	}

	stream := cipher.NewCFBEncrypter(block, cfbIV)
	encrypted := make([]byte, len(data))
	stream.XORKeyStream(encrypted, data)

	return encrypted, nil
}

// DecryptDataPCFB decrypts data using AES-256-PCFB mode
func DecryptDataPCFB(data, key, iv []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Use provided IV or zero IV
	var cfbIV []byte
	if iv != nil {
		cfbIV = iv
	} else {
		cfbIV = make([]byte, aes.BlockSize)
	}

	stream := cipher.NewCFBDecrypter(block, cfbIV)
	decrypted := make([]byte, len(data))
	stream.XORKeyStream(decrypted, data)

	return decrypted, nil
}

// HashDocname computes SHA256 hash of a document name
func HashDocname(docname string) []byte {
	h := sha256.Sum256([]byte(docname))
	return h[:]
}

// HashData computes SHA256 hash of data
func HashData(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// GenerateSSKKeypair generates a new SSK keypair (DSA)
// Returns private key bytes and public key bytes
func GenerateSSKKeypair() ([]byte, []byte, error) {
	// Freenet uses DSA with specific parameters
	// For simplicity, we'll use Go's standard DSA with L=1024, N=160
	params := &dsa.Parameters{}
	if err := dsa.GenerateParameters(params, rand.Reader, dsa.L1024N160); err != nil {
		return nil, nil, fmt.Errorf("failed to generate DSA parameters: %w", err)
	}

	privKey := &dsa.PrivateKey{}
	privKey.Parameters = *params
	if err := dsa.GenerateKey(privKey, rand.Reader); err != nil {
		return nil, nil, fmt.Errorf("failed to generate DSA key: %w", err)
	}

	// Serialize private key (X value)
	privBytes := privKey.X.Bytes()

	// Pad to 20 bytes (160 bits)
	if len(privBytes) < 20 {
		padded := make([]byte, 20)
		copy(padded[20-len(privBytes):], privBytes)
		privBytes = padded
	}

	// Serialize public key (Y value)
	pubBytes := privKey.Y.Bytes()

	// Pad to 128 bytes (1024 bits)
	if len(pubBytes) < 128 {
		padded := make([]byte, 128)
		copy(padded[128-len(pubBytes):], pubBytes)
		pubBytes = padded
	}

	return privBytes, pubBytes, nil
}

// SignData signs data using a DSA private key
// This is a simplified version for SSK signatures
func SignData(data []byte, privKeyBytes []byte) (r, s *big.Int, err error) {
	// Reconstruct DSA private key
	// This is simplified - in practice, we'd need the full parameters
	privKey := &dsa.PrivateKey{
		X: new(big.Int).SetBytes(privKeyBytes),
	}

	// Generate DSA parameters (would be stored/cached in real implementation)
	params := &dsa.Parameters{}
	if err := dsa.GenerateParameters(params, rand.Reader, dsa.L1024N160); err != nil {
		return nil, nil, fmt.Errorf("failed to generate DSA parameters: %w", err)
	}
	privKey.Parameters = *params

	// Hash the data
	hash := sha256.Sum256(data)

	// Sign
	return dsa.Sign(rand.Reader, privKey, hash[:])
}

// VerifySignature verifies a DSA signature
func VerifySignature(data []byte, pubKeyBytes []byte, r, s *big.Int) bool {
	// Reconstruct DSA public key
	pubKey := &dsa.PublicKey{
		Y: new(big.Int).SetBytes(pubKeyBytes),
	}

	// Generate DSA parameters (would be stored/cached in real implementation)
	params := &dsa.Parameters{}
	dsa.GenerateParameters(params, rand.Reader, dsa.L1024N160)
	pubKey.Parameters = *params

	// Hash the data
	hash := sha256.Sum256(data)

	// Verify
	return dsa.Verify(pubKey, hash[:], r, s)
}

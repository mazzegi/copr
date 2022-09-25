package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/pkg/errors"
)

func generateKey(pwd string) ([]byte, error) {
	hash := sha256.Sum256([]byte(pwd))
	return hash[:], nil
}

func encrypt(data []byte, pwd string) ([]byte, error) {
	key, err := generateKey(pwd)
	if err != nil {
		return nil, errors.Wrap(err, "generate-key")
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new cipher")
	}
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "cipher: new GCM")
	}
	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, errors.Wrap(err, "read to nonce")
	}

	cipherdata := gcm.Seal(nonce, nonce, data, nil)
	return cipherdata, nil
}

func decrypt(ciphertext []byte, pwd string) ([]byte, error) {
	key, err := generateKey(pwd)
	if err != nil {
		return nil, errors.Wrap(err, "generate-key")
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "new-cipher")
	}
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "new-GCM")
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.Errorf("ciphertext is less than nonce-size")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaindata, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "gcm-open")
	}
	return plaindata, nil
}

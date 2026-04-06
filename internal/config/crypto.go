package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// 盐值长度
	saltLen = 32
	// AES-256 密钥长度
	keyLen = 32
	// PBKDF2 迭代次数
	pbkdf2Iter = 100000
)

// Encryptor 凭证加密器，使用 AES-256-GCM
type Encryptor struct {
	key []byte
}

// NewEncryptor 从机器指纹 + 用户密码派生加密密钥
// machineID 可以使用机器 UUID，password 为空则仅使用机器指纹
func NewEncryptor(machineID, password string) *Encryptor {
	combined := machineID + "::voyage-salt::" + password
	salt := sha256.Sum256([]byte(combined + "-pbkdf2-salt"))
	key := pbkdf2.Key([]byte(combined), salt[:], pbkdf2Iter, keyLen, sha256.New)
	return &Encryptor{key: key}
}

// Encrypt 加密明文，返回 [salt(32) + nonce(12) + ciphertext] 格式
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt 解密密文
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("密文长度不足")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("解密失败：凭证数据已损坏或密钥错误")
	}

	return plaintext, nil
}

// EncryptString 加密字符串便捷方法
func (e *Encryptor) EncryptString(s string) ([]byte, error) {
	return e.Encrypt([]byte(s))
}

// DecryptString 解密为字符串便捷方法
func (e *Encryptor) DecryptString(ciphertext []byte) (string, error) {
	b, err := e.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	enc := NewEncryptor("mock-machine-id", "mock-password")
	plaintext := []byte("hello world for voyage platform")

	// Encrypt
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt
	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Expected %s, got %s", plaintext, decrypted)
	}
}

func TestEncryptDecryptString(t *testing.T) {
	enc := NewEncryptor("mock-id", "pass")
	plaintext := "Voyage Security Test Token 123"

	ciphertext, err := enc.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	decrypted, err := enc.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}

	if plaintext != decrypted {
		t.Errorf("Expected %s, got %s", plaintext, decrypted)
	}
}

func TestDecryptErrors(t *testing.T) {
	enc := NewEncryptor("mock-id", "pass")

	// Empty ciphertext or short ciphertext
	_, err := enc.Decrypt([]byte("short"))
	if err == nil {
		t.Error("Expected error on short ciphertext")
	} else if !strings.Contains(err.Error(), "密文长度不足") {
		t.Errorf("Unexpected error message for short ciphertext: %v", err)
	}

	// Corrupt ciphertext
	plaintext := "secret vault data"
	ciphertext, _ := enc.EncryptString(plaintext)
	
	// Alter the last byte to invalidate GCM auth tag
	ciphertext[len(ciphertext)-1] ^= 0xff
	
	_, err = enc.DecryptString(ciphertext)
	if err == nil {
		t.Error("Expected error on corrupted ciphertext")
	} else if !strings.Contains(err.Error(), "解密失败") {
		t.Errorf("Unexpected error message for corrupted ciphertext: %v", err)
	}
}

func TestDifferentKeys(t *testing.T) {
	enc1 := NewEncryptor("mock-id-1", "")
	enc2 := NewEncryptor("mock-id-2", "")

	plaintext := "hello"
	ciphertext, _ := enc1.EncryptString(plaintext)

	// Try decrypting with wrong key
	_, err := enc2.DecryptString(ciphertext)
	if err == nil {
		t.Error("Expected decryption to fail with wrong key")
	}
}

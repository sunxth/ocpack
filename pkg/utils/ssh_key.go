package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// EnsureSSHKeyPair 确保SSH密钥对存在，如果不存在则创建
func EnsureSSHKeyPair() (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return "", fmt.Errorf("无法获取用户主目录")
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	privateKeyPath := filepath.Join(sshDir, "id_rsa")
	publicKeyPath := filepath.Join(sshDir, "id_rsa.pub")

	// 检查公钥是否存在
	if _, err := os.Stat(publicKeyPath); err == nil {
		// 读取并返回公钥内容
		publicKeyContent, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return "", fmt.Errorf("读取SSH公钥失败: %v", err)
		}
		return string(publicKeyContent), nil
	}

	// 如果公钥不存在，创建新的密钥对
	fmt.Printf("🔑 生成SSH密钥对...\n")
	return generateSSHKeyPair(privateKeyPath, publicKeyPath)
}

// generateSSHKeyPair 生成新的SSH密钥对
func generateSSHKeyPair(privateKeyPath, publicKeyPath string) (string, error) {
	// 确保.ssh目录存在
	sshDir := filepath.Dir(privateKeyPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", fmt.Errorf("创建.ssh目录失败: %v", err)
	}

	// 生成RSA私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("生成RSA私钥失败: %v", err)
	}

	// 编码私钥为PEM格式
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	// 写入私钥文件
	privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("创建私钥文件失败: %v", err)
	}
	defer privateKeyFile.Close()

	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return "", fmt.Errorf("写入私钥失败: %v", err)
	}

	// 生成SSH公钥
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("生成SSH公钥失败: %v", err)
	}

	// 格式化公钥
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)
	publicKeyContent := string(publicKeyBytes)

	// 写入公钥文件
	if err := os.WriteFile(publicKeyPath, []byte(publicKeyContent), 0644); err != nil {
		return "", fmt.Errorf("写入公钥文件失败: %v", err)
	}

	fmt.Printf("✅ SSH密钥对已生成\n")

	return publicKeyContent, nil
}

// GetSSHPublicKey 获取SSH公钥内容，如果不存在则创建
func GetSSHPublicKey() (string, error) {
	publicKeyContent, err := EnsureSSHKeyPair()
	if err != nil {
		return "", err
	}

	// 去除换行符并返回
	return strings.TrimSpace(publicKeyContent), nil
}

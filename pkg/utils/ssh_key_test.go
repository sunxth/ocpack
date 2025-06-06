package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSSHKeyPair(t *testing.T) {
	// 创建临时目录作为测试用的HOME目录
	tempDir, err := os.MkdirTemp("", "ssh-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 设置临时HOME环境变量
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// 测试生成新的SSH密钥对
	publicKey, err := EnsureSSHKeyPair()
	if err != nil {
		t.Fatalf("生成SSH密钥对失败: %v", err)
	}

	if publicKey == "" {
		t.Fatal("返回的公钥内容为空")
	}

	// 验证文件是否创建
	sshDir := filepath.Join(tempDir, ".ssh")
	privateKeyPath := filepath.Join(sshDir, "id_rsa")
	publicKeyPath := filepath.Join(sshDir, "id_rsa.pub")

	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		t.Fatal("私钥文件未创建")
	}

	if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
		t.Fatal("公钥文件未创建")
	}

	// 验证公钥格式
	if !strings.HasPrefix(publicKey, "ssh-rsa ") {
		t.Fatalf("公钥格式不正确，应该以'ssh-rsa '开头，实际: %s", publicKey[:20])
	}

	// 测试再次调用时使用现有密钥
	publicKey2, err := EnsureSSHKeyPair()
	if err != nil {
		t.Fatalf("使用现有SSH密钥失败: %v", err)
	}

	if publicKey != publicKey2 {
		t.Fatal("两次调用返回的公钥内容不一致")
	}
}

func TestGetSSHPublicKey(t *testing.T) {
	// 创建临时目录作为测试用的HOME目录
	tempDir, err := os.MkdirTemp("", "ssh-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 设置临时HOME环境变量
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// 测试获取SSH公钥
	publicKey, err := GetSSHPublicKey()
	if err != nil {
		t.Fatalf("获取SSH公钥失败: %v", err)
	}

	if publicKey == "" {
		t.Fatal("返回的公钥内容为空")
	}

	// 验证公钥格式
	if !strings.HasPrefix(publicKey, "ssh-rsa ") {
		t.Fatalf("公钥格式不正确，应该以'ssh-rsa '开头，实际: %s", publicKey[:20])
	}

	// 验证公钥内容不包含换行符
	if strings.Contains(publicKey, "\n") {
		t.Fatal("公钥内容不应包含换行符")
	}
}

func TestEnsureSSHKeyPairNoHomeDir(t *testing.T) {
	// 保存原始HOME环境变量
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// 清空HOME环境变量
	os.Unsetenv("HOME")

	// 测试在没有HOME目录的情况下
	_, err := EnsureSSHKeyPair()
	if err == nil {
		t.Fatal("在没有HOME目录的情况下应该返回错误")
	}

	if !strings.Contains(err.Error(), "无法获取用户主目录") {
		t.Fatalf("错误信息不正确，期望包含'无法获取用户主目录'，实际: %v", err)
	}
}

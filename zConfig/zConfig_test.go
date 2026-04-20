package zConfig

import (
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	filePath := "test_config.json"

	config := NewConfig()

	err := config.Set("server.host", "localhost")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("server.port", 8080)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("server.debug", true)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("database.url", "mysql://user:pass@localhost:3306/db")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.SaveJSON(filePath)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	newConfig := NewConfig()

	err = newConfig.LoadJSON(filePath)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	host, err := newConfig.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if host != "localhost" {
		t.Errorf("Host mismatch: expected 'localhost', got '%s'", host)
	}

	port, err := newConfig.GetInt("server.port")
	if err != nil {
		t.Fatalf("GetInt failed: %v", err)
	}

	if port != 8080 {
		t.Errorf("Port mismatch: expected 8080, got %d", port)
	}

	debug, err := newConfig.GetBool("server.debug")
	if err != nil {
		t.Fatalf("GetBool failed: %v", err)
	}

	if !debug {
		t.Errorf("Debug mismatch: expected true, got false")
	}

	dbURL, err := newConfig.GetString("database.url")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if dbURL != "mysql://user:pass@localhost:3306/db" {
		t.Errorf("DB URL mismatch: expected 'mysql://user:pass@localhost:3306/db', got '%s'", dbURL)
	}

	if !newConfig.Has("server.host") {
		t.Errorf("Has failed: expected true for 'server.host'")
	}

	if newConfig.Has("nonexistent.key") {
		t.Errorf("Has failed: expected false for 'nonexistent.key'")
	}

	err = newConfig.Delete("server.debug")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if newConfig.Has("server.debug") {
		t.Errorf("Delete failed: 'server.debug' still exists")
	}

	type ServerConfig struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	type DatabaseConfig struct {
		URL string `json:"url"`
	}

	type AppConfig struct {
		Server   ServerConfig   `json:"server"`
		Database DatabaseConfig `json:"database"`
	}

	var appConfig AppConfig
	err = newConfig.Unmarshal(&appConfig)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if appConfig.Server.Host != "localhost" {
		t.Errorf("Unmarshal: Host mismatch: expected 'localhost', got '%s'", appConfig.Server.Host)
	}

	if appConfig.Server.Port != 8080 {
		t.Errorf("Unmarshal: Port mismatch: expected 8080, got %d", appConfig.Server.Port)
	}

	if appConfig.Database.URL != "mysql://user:pass@localhost:3306/db" {
		t.Errorf("Unmarshal: DB URL mismatch: expected 'mysql://user:pass@localhost:3306/db', got '%s'", appConfig.Database.URL)
	}

	newAppConfig := AppConfig{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 9090,
		},
		Database: DatabaseConfig{
			URL: "postgres://user:pass@localhost:5432/db",
		},
	}

	err = config.Marshal(newAppConfig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	newHost, err := config.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if newHost != "127.0.0.1" {
		t.Errorf("Marshal: Host mismatch: expected '127.0.0.1', got '%s'", newHost)
	}

	config.Clear()
	if !config.IsEmpty() {
		t.Errorf("Clear failed: config is not empty")
	}

	os.Remove(filePath)
}

func TestConfigYAML(t *testing.T) {
	filePath := "test_config.yaml"

	config := NewConfig()

	err := config.Set("server.host", "localhost")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("server.port", 8080)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("server.debug", true)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.Set("database.url", "mysql://user:pass@localhost:3306/db")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = config.SaveYAML(filePath)
	if err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}

	newConfig := NewConfig()

	err = newConfig.LoadYAML(filePath)
	if err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	host, err := newConfig.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if host != "localhost" {
		t.Errorf("Host mismatch: expected 'localhost', got '%s'", host)
	}

	os.Remove(filePath)
}

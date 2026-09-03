package zConfig

import (
	"os"
	"path/filepath"
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

func TestGetIntSliceWithDefaultAcceptsScalarINIValue(t *testing.T) {
	filePath := "test_config_slice.ini"
	if err := os.WriteFile(filePath, []byte("[Maps]\nMapIDs = 1002\n"), 0644); err != nil {
		t.Fatalf("write INI: %v", err)
	}
	defer os.Remove(filePath)

	cfg := NewConfig()
	if err := cfg.LoadINI(filePath); err != nil {
		t.Fatalf("LoadINI: %v", err)
	}
	got := GetIntSliceWithDefault(cfg, "Maps.MapIDs", []int{1001})
	if len(got) != 1 || got[0] != 1002 {
		t.Fatalf("scalar int slice=%v, want [1002]", got)
	}
}

func TestINIPreservesTextAndTypedGettersParseOnDemand(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.ini")
	content := []byte("[Secrets]\nPassword = 123456\nToken = 001234\n\n[Scalars]\nInteger = 42\nFloat = 3.25\nEnabled = TRUE\nInvalidInteger = 12x\n")
	if err := os.WriteFile(input, content, 0644); err != nil {
		t.Fatalf("write INI: %v", err)
	}

	cfg := NewConfig()
	if err := cfg.LoadINI(input); err != nil {
		t.Fatalf("LoadINI: %v", err)
	}

	for key, want := range map[string]string{
		"Secrets.Password": "123456",
		"Secrets.Token":    "001234",
		"Scalars.Integer":  "42",
		"Scalars.Float":    "3.25",
		"Scalars.Enabled":  "TRUE",
	} {
		got, err := cfg.Get(key)
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if got != want {
			t.Fatalf("Get(%q)=%#v (%T), want raw string %q", key, got, got, want)
		}
	}

	if got, err := cfg.GetString("Secrets.Token"); err != nil || got != "001234" {
		t.Fatalf("GetString(Token)=%q, %v; want preserved 001234", got, err)
	}
	if got, err := cfg.GetInt("Scalars.Integer"); err != nil || got != 42 {
		t.Fatalf("GetInt(Integer)=%d, %v; want 42", got, err)
	}
	if got, err := cfg.GetFloat("Scalars.Float"); err != nil || got != 3.25 {
		t.Fatalf("GetFloat(Float)=%v, %v; want 3.25", got, err)
	}
	if got, err := cfg.GetBool("Scalars.Enabled"); err != nil || !got {
		t.Fatalf("GetBool(Enabled)=%v, %v; want true", got, err)
	}
	if _, err := cfg.GetInt("Scalars.InvalidInteger"); err == nil {
		t.Fatal("GetInt(InvalidInteger) should reject non-integer text")
	}
	if got := GetIntWithDefault(cfg, "Scalars.InvalidInteger", 17); got != 17 {
		t.Fatalf("invalid integer default=%d, want 17", got)
	}

	output := filepath.Join(dir, "output.ini")
	if err := cfg.SaveINI(output); err != nil {
		t.Fatalf("SaveINI: %v", err)
	}
	reloaded := NewConfig()
	if err := reloaded.LoadINI(output); err != nil {
		t.Fatalf("reload saved INI: %v", err)
	}
	if got, err := reloaded.GetString("Secrets.Token"); err != nil || got != "001234" {
		t.Fatalf("saved Token=%q, %v; want preserved 001234", got, err)
	}
}

func TestINIUnmarshalUsesDestinationTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	content := []byte("[Secrets]\nPassword = 123456\nToken = 001234\n\n[Scalars]\nInteger = 42\nFloat = 3.25\nEnabled = true\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write INI: %v", err)
	}

	cfg := NewConfig()
	if err := cfg.LoadINI(path); err != nil {
		t.Fatalf("LoadINI: %v", err)
	}

	type secrets struct {
		Password string `ini:"Password"`
		Token    string `ini:"Token"`
	}
	type scalars struct {
		Integer int     `ini:"Integer"`
		Float   float64 `ini:"Float"`
		Enabled bool    `ini:"Enabled"`
	}
	var target struct {
		Secrets secrets `ini:"Secrets"`
		Scalars scalars `ini:"Scalars"`
	}
	if err := cfg.Unmarshal(&target); err != nil {
		t.Fatalf("Unmarshal INI: %v", err)
	}
	if target.Secrets.Password != "123456" || target.Secrets.Token != "001234" {
		t.Fatalf("unmarshaled secrets=%+v, want numeric text preserved", target.Secrets)
	}
	if target.Scalars.Integer != 42 || target.Scalars.Float != 3.25 || !target.Scalars.Enabled {
		t.Fatalf("unmarshaled scalars=%+v", target.Scalars)
	}

	var raw map[string]interface{}
	if err := cfg.Unmarshal(&raw); err != nil {
		t.Fatalf("Unmarshal raw INI map: %v", err)
	}
	rawSecrets, ok := raw["Secrets"].(map[string]interface{})
	if !ok || rawSecrets["Token"] != "001234" {
		t.Fatalf("raw unmarshaled secrets=%#v, want string token", raw["Secrets"])
	}

	if err := cfg.Set("Scalars.Integer", "not-an-integer"); err != nil {
		t.Fatalf("Set invalid integer: %v", err)
	}
	var invalidTarget struct {
		Scalars scalars `ini:"Scalars"`
	}
	if err := cfg.Unmarshal(&invalidTarget); err == nil {
		t.Fatal("strict INI Unmarshal should reject invalid destination scalar")
	}
}

func TestJSONAndYAMLKeepNativeScalarTypes(t *testing.T) {
	dir := t.TempDir()

	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"Scalars":{"Integer":42,"Float":3.25,"Enabled":true,"Token":"001234"}}`), 0644); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
	jsonConfig := NewConfig()
	if err := jsonConfig.LoadJSON(jsonPath); err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if got, err := jsonConfig.Get("Scalars.Integer"); err != nil || got != float64(42) {
		t.Fatalf("JSON integer=%#v (%T), %v; want native float64", got, got, err)
	}
	if _, err := jsonConfig.GetString("Scalars.Integer"); err == nil {
		t.Fatal("JSON number should not be coerced to string")
	}
	if got, err := jsonConfig.GetInt("Scalars.Integer"); err != nil || got != 42 {
		t.Fatalf("JSON GetInt=%d, %v; want 42", got, err)
	}
	if got, err := jsonConfig.GetFloat("Scalars.Float"); err != nil || got != 3.25 {
		t.Fatalf("JSON GetFloat=%v, %v; want 3.25", got, err)
	}
	if got, err := jsonConfig.GetBool("Scalars.Enabled"); err != nil || !got {
		t.Fatalf("JSON GetBool=%v, %v; want true", got, err)
	}
	if got, err := jsonConfig.GetString("Scalars.Token"); err != nil || got != "001234" {
		t.Fatalf("JSON token=%q, %v", got, err)
	}

	yamlPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("Scalars:\n  Integer: 42\n  Float: 3.25\n  Enabled: true\n  Token: '001234'\n"), 0644); err != nil {
		t.Fatalf("write YAML: %v", err)
	}
	yamlConfig := NewConfig()
	if err := yamlConfig.LoadYAML(yamlPath); err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if got, err := yamlConfig.Get("Scalars.Integer"); err != nil || got != 42 {
		t.Fatalf("YAML integer=%#v (%T), %v; want native int", got, got, err)
	}
	if _, err := yamlConfig.GetString("Scalars.Integer"); err == nil {
		t.Fatal("YAML number should not be coerced to string")
	}
	if got, err := yamlConfig.GetInt("Scalars.Integer"); err != nil || got != 42 {
		t.Fatalf("YAML GetInt=%d, %v; want 42", got, err)
	}
	if got, err := yamlConfig.GetFloat("Scalars.Float"); err != nil || got != 3.25 {
		t.Fatalf("YAML GetFloat=%v, %v; want 3.25", got, err)
	}
	if got, err := yamlConfig.GetBool("Scalars.Enabled"); err != nil || !got {
		t.Fatalf("YAML GetBool=%v, %v; want true", got, err)
	}
	if got, err := yamlConfig.GetString("Scalars.Token"); err != nil || got != "001234" {
		t.Fatalf("YAML token=%q, %v", got, err)
	}
}

func TestEnvironmentTextAndDefaultsRemainExact(t *testing.T) {
	const key = "ZCONFIG_CFG01_TOKEN"
	t.Setenv(key, "001234")
	if got := GetEnv(key, "fallback"); got != "001234" {
		t.Fatalf("GetEnv=%q, want 001234", got)
	}

	const missing = "ZCONFIG_CFG01_MISSING"
	if err := os.Unsetenv(missing); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	if got := GetEnv(missing, "000777"); got != "000777" {
		t.Fatalf("missing env default=%q, want 000777", got)
	}

	const empty = "ZCONFIG_CFG01_EMPTY"
	t.Setenv(empty, "")
	if got := GetEnv(empty, "fallback"); got != "" {
		t.Fatalf("present empty env=%q, want empty string", got)
	}
}

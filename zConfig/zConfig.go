package zConfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

// Config 配置结构体
type Config struct {
	data   map[string]interface{}
	path   string
	source configSource
}

type configSource uint8

const (
	configSourceUnknown configSource = iota
	configSourceJSON
	configSourceYAML
	configSourceINI
)

// NewConfig 创建一个新的配置实例
func NewConfig() *Config {
	return &Config{
		data: make(map[string]interface{}),
	}
}

// LoadJSON 从JSON文件加载配置
func (c *Config) LoadJSON(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	err = json.Unmarshal(content, &data)
	if err != nil {
		return err
	}

	c.data = data
	c.path = filePath
	c.source = configSourceJSON
	return nil
}

// SaveJSON 保存配置到JSON文件
func (c *Config) SaveJSON(filePath string) error {
	content, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, content, 0644)
	if err != nil {
		return err
	}

	c.path = filePath
	return nil
}

// LoadYAML 从YAML文件加载配置
func (c *Config) LoadYAML(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	err = yaml.Unmarshal(content, &data)
	if err != nil {
		return err
	}

	c.data = data
	c.path = filePath
	c.source = configSourceYAML
	return nil
}

// SaveYAML 保存配置到YAML文件
func (c *Config) SaveYAML(filePath string) error {
	content, err := yaml.Marshal(c.data)
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, content, 0644)
	if err != nil {
		return err
	}

	c.path = filePath
	return nil
}

// LoadINI 从INI文件加载配置
func (c *Config) LoadINI(filePath string) error {
	cfg, err := ini.Load(filePath)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})

	defaultSection := cfg.Section("")
	if defaultSection != nil {
		sectionMap := make(map[string]interface{})
		for _, key := range defaultSection.Keys() {
			// INI has no scalar schema. Keep source text until a typed getter or Unmarshal selects a type.
			sectionMap[key.Name()] = key.Value()
		}
		if len(sectionMap) > 0 {
			for k, v := range sectionMap {
				data[k] = v
			}
		}
	}

	for _, section := range cfg.Sections() {
		if section.Name() == "" {
			continue
		}

		sectionMap := make(map[string]interface{})
		for _, key := range section.Keys() {
			sectionMap[key.Name()] = key.Value()
		}

		sectionKeys := strings.Split(section.Name(), ".")
		currentMap := data

		for i, k := range sectionKeys {
			if i == len(sectionKeys)-1 {
				currentMap[k] = sectionMap
			} else {
				if currentMap[k] == nil {
					currentMap[k] = make(map[string]interface{})
				}
				var ok bool
				currentMap, ok = currentMap[k].(map[string]interface{})
				if !ok {
					currentMap = make(map[string]interface{})
					currentMap[sectionKeys[i]] = sectionMap
					break
				}
			}
		}
	}

	c.data = data
	c.path = filePath
	c.source = configSourceINI
	return nil
}

// SaveINI 保存配置到INI文件
func (c *Config) SaveINI(filePath string) error {
	cfg := ini.Empty()

	for key, value := range c.data {
		if sectionMap, ok := value.(map[string]interface{}); ok {
			section := cfg.Section(key)
			for k, v := range sectionMap {
				section.Key(k).SetValue(convertToINIString(v))
			}
		} else {
			cfg.Section("").Key(key).SetValue(convertToINIString(value))
		}
	}

	err := cfg.SaveTo(filePath)
	if err != nil {
		return err
	}

	c.path = filePath
	return nil
}

// convertToINIString 将值转换为INI字符串
func convertToINIString(value interface{}) string {
	switch v := value.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return v
	default:
		return ""
	}
}

// Get 获取配置值
func (c *Config) Get(key string) (interface{}, error) {
	keys := strings.Split(key, ".")
	var value interface{}
	value = c.data

	for _, k := range keys {
		m, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.New("invalid config path: " + key)
		}

		value, ok = m[k]
		if !ok {
			return nil, errors.New("key not found: " + key)
		}
	}

	return value, nil
}

// GetString 获取字符串类型的配置值
func (c *Config) GetString(key string) (string, error) {
	value, err := c.Get(key)
	if err != nil {
		return "", err
	}

	str, ok := value.(string)
	if !ok {
		return "", errors.New("value is not a string: " + key)
	}

	return str, nil
}

// GetInt 获取整数类型的配置值
func (c *Config) GetInt(key string) (int, error) {
	value, err := c.Get(key)
	if err != nil {
		return 0, err
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed, nil
		}
	case float64:
		return int(v), nil
	case int64:
		return int(v), nil
	}
	return 0, errors.New("value is not an integer: " + key)
}

// GetFloat 获取浮点数类型的配置值
func (c *Config) GetFloat(key string) (float64, error) {
	value, err := c.Get(key)
	if err != nil {
		return 0, err
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed, nil
		}
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	}
	return 0, errors.New("value is not a float: " + key)
}

// GetBool 获取布尔类型的配置值
func (c *Config) GetBool(key string) (bool, error) {
	value, err := c.Get(key)
	if err != nil {
		return false, err
	}

	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed, nil
		}
	}

	return false, errors.New("value is not a boolean: " + key)
}

// Set 设置配置值
func (c *Config) Set(key string, value interface{}) error {
	keys := strings.Split(key, ".")
	var current interface{}
	current = c.data

	for i, k := range keys {
		if i == len(keys)-1 {
			m, ok := current.(map[string]interface{})
			if !ok {
				return errors.New("invalid config path: " + key)
			}
			m[k] = value
			return nil
		}

		m, ok := current.(map[string]interface{})
		if !ok {
			return errors.New("invalid config path: " + key)
		}

		next, ok := m[k]
		if !ok {
			newMap := make(map[string]interface{})
			m[k] = newMap
			next = newMap
		}

		_, ok = next.(map[string]interface{})
		if !ok {
			return errors.New("invalid config path: " + key)
		}

		current = next
	}

	return nil
}

// Unmarshal 将配置解析到结构体
func (c *Config) Unmarshal(v interface{}) error {
	if c.source == configSourceINI && isINIStructTarget(v) {
		cfg := ini.Empty()
		populateINI(cfg, "", c.data)
		return cfg.StrictMapTo(v)
	}

	content, err := json.Marshal(c.data)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, v)
}

// Marshal 将结构体解析到配置
func (c *Config) Marshal(v interface{}) error {
	content, err := json.Marshal(v)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	err = json.Unmarshal(content, &data)
	if err != nil {
		return err
	}

	c.data = data
	c.source = configSourceJSON
	return nil
}

func isINIStructTarget(v interface{}) bool {
	value := reflect.ValueOf(v)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return false
	}
	kind := value.Elem().Kind()
	return kind == reflect.Struct || kind == reflect.Slice
}

func populateINI(cfg *ini.File, sectionName string, values map[string]interface{}) {
	section := cfg.Section(sectionName)
	for key, value := range values {
		if child, ok := value.(map[string]interface{}); ok {
			childSection := key
			if sectionName != "" {
				childSection = sectionName + "." + key
			}
			populateINI(cfg, childSection, child)
			continue
		}
		section.Key(key).SetValue(convertToINIString(value))
	}
}

// Has 检查配置中是否存在某个键
func (c *Config) Has(key string) bool {
	_, err := c.Get(key)
	return err == nil
}

// Delete 删除配置中的某个键
func (c *Config) Delete(key string) error {
	keys := strings.Split(key, ".")
	var current interface{}
	current = c.data

	for i, k := range keys {
		if i == len(keys)-1 {
			m, ok := current.(map[string]interface{})
			if !ok {
				return errors.New("invalid config path: " + key)
			}
			delete(m, k)
			return nil
		}

		m, ok := current.(map[string]interface{})
		if !ok {
			return errors.New("invalid config path: " + key)
		}

		next, ok := m[k]
		if !ok {
			return errors.New("key not found: " + key)
		}

		_, ok = next.(map[string]interface{})
		if !ok {
			return errors.New("invalid config path: " + key)
		}

		current = next
	}

	return nil
}

// Clear 清空配置
func (c *Config) Clear() {
	c.data = make(map[string]interface{})
	c.source = configSourceUnknown
}

// Size 获取配置中的键值对数量
func (c *Config) Size() int {
	return len(c.data)
}

// IsEmpty 检查配置是否为空
func (c *Config) IsEmpty() bool {
	return len(c.data) == 0
}

// GetStringWithDefault 获取字符串配置值，不存在则返回默认值
func GetStringWithDefault(cfg *Config, key string, defaultValue string) string {
	if value, err := cfg.GetString(key); err == nil {
		return value
	}
	return defaultValue
}

// GetIntWithDefault 获取整数配置值，不存在则返回默认值
func GetIntWithDefault(cfg *Config, key string, defaultValue int) int {
	if value, err := cfg.GetInt(key); err == nil {
		return value
	}
	return defaultValue
}

// GetBoolWithDefault 获取布尔配置值，不存在则返回默认值
func GetBoolWithDefault(cfg *Config, key string, defaultValue bool) bool {
	if value, err := cfg.GetBool(key); err == nil {
		return value
	}
	return defaultValue
}

// GetFloatWithDefault 获取浮点数配置值，不存在则返回默认值
func GetFloatWithDefault(cfg *Config, key string, defaultValue float64) float64 {
	if value, err := cfg.GetFloat(key); err == nil {
		return value
	}
	return defaultValue
}

// GetIntSliceWithDefault 获取整数切片配置值，不存在则返回默认值
func GetIntSliceWithDefault(cfg *Config, key string, defaultValue []int) []int {
	if cfg == nil {
		return defaultValue
	}
	value, err := cfg.Get(key)
	if err != nil {
		return defaultValue
	}
	if scalar, ok := value.(int); ok {
		return []int{scalar}
	}
	if scalar, ok := value.(int64); ok {
		return []int{int(scalar)}
	}
	if scalar, ok := value.(float64); ok && scalar == float64(int(scalar)) {
		return []int{int(scalar)}
	}
	if text, ok := value.(string); ok {
		strs := strings.Split(text, ",")
		ints := make([]int, 0, len(strs))
		for _, str := range strs {
			str = strings.TrimSpace(str)
			if str != "" {
				if i, err := strconv.Atoi(str); err == nil {
					ints = append(ints, i)
				}
			}
		}
		if len(ints) > 0 {
			return ints
		}
	}
	return defaultValue
}

// GetEnv 获取环境变量，不存在则返回默认值
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetEnvAsInt 获取环境变量为整数，不存在则返回默认值
func GetEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvAsBool 获取环境变量为布尔值，不存在则返回默认值
func GetEnvAsBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// ReplacePlaceholder 替换字符串中的占位符
func ReplacePlaceholder(s, placeholder string, value int) string {
	return strings.Replace(s, placeholder, fmt.Sprintf("%d", value), -1)
}

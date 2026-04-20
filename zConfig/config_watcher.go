package zConfig

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Logger interface {
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Debug(msg string, fields ...Field)
}

type Field struct {
	Key string
	Val interface{}
}

func StrField(key, val string) Field {
	return Field{Key: key, Val: val}
}

func IntField(key string, val int) Field {
	return Field{Key: key, Val: val}
}

func Int64Field(key string, val int64) Field {
	return Field{Key: key, Val: val}
}

func ErrField(err error) Field {
	return Field{Key: "error", Val: err}
}

func AnyField(key string, val interface{}) Field {
	return Field{Key: key, Val: val}
}

type noopLogger struct{}

func (noopLogger) Info(string, ...Field)  {}
func (noopLogger) Warn(string, ...Field)  {}
func (noopLogger) Error(string, ...Field) {}
func (noopLogger) Debug(string, ...Field) {}

var defaultLogger Logger = noopLogger{}

func SetLogger(l Logger) {
	if l != nil {
		defaultLogger = l
	}
}

type ConfigChangeCallback func(key string, oldValue, newValue []byte)

type WatchConfig struct {
	Prefix     string
	EtcdConfig EtcdWatchConfig
	PollTick   time.Duration
}

type EtcdWatchConfig struct {
	Endpoints      []string
	Username       string
	Password       string
	ConnectTimeout time.Duration
}

func DefaultWatchConfig(prefix string) WatchConfig {
	return WatchConfig{
		Prefix: prefix,
		EtcdConfig: EtcdWatchConfig{
			Endpoints:      []string{"localhost:2379"},
			ConnectTimeout: 5 * time.Second,
		},
		PollTick: 10 * time.Second,
	}
}

type ConfigEntry struct {
	Key       string
	Value     []byte
	Version   int64
	UpdatedAt time.Time
}

type ConfigWatcher struct {
	etcdClient *clientv3.Client
	config     WatchConfig
	entries    *zMap.TypedMap[string, *ConfigEntry]
	callbacks  *zMap.TypedMap[string, []ConfigChangeCallback]
	running    atomic.Bool
	mu         sync.Mutex
	logger     Logger
}

func NewConfigWatcher(config WatchConfig) *ConfigWatcher {
	return &ConfigWatcher{
		config:    config,
		entries:   zMap.NewTypedMap[string, *ConfigEntry](),
		callbacks: zMap.NewTypedMap[string, []ConfigChangeCallback](),
		logger:    defaultLogger,
	}
}

func (cw *ConfigWatcher) SetLogger(l Logger) {
	if l != nil {
		cw.logger = l
	}
}

func (cw *ConfigWatcher) Start(ctx context.Context) error {
	if cw.running.Load() {
		return fmt.Errorf("config watcher already running")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cw.config.EtcdConfig.Endpoints,
		Username:    cw.config.EtcdConfig.Username,
		Password:    cw.config.EtcdConfig.Password,
		DialTimeout: cw.config.EtcdConfig.ConnectTimeout,
	})
	if err != nil {
		return fmt.Errorf("connect to etcd: %w", err)
	}

	cw.etcdClient = client

	if err := cw.loadInitial(ctx); err != nil {
		cw.logger.Warn("Failed to load initial config, will retry on watch",
			StrField("prefix", cw.config.Prefix),
			ErrField(err))
	}

	cw.running.Store(true)

	go cw.watchLoop(ctx)

	cw.logger.Info("Config watcher started",
		StrField("prefix", cw.config.Prefix),
		AnyField("endpoints", cw.config.EtcdConfig.Endpoints))

	return nil
}

func (cw *ConfigWatcher) Stop() {
	cw.running.Store(false)
	if cw.etcdClient != nil {
		cw.etcdClient.Close()
	}
	cw.logger.Info("Config watcher stopped", StrField("prefix", cw.config.Prefix))
}

func (cw *ConfigWatcher) loadInitial(ctx context.Context) error {
	resp, err := cw.etcdClient.Get(ctx, cw.config.Prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("etcd get prefix %s: %w", cw.config.Prefix, err)
	}

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		entry := &ConfigEntry{
			Key:       key,
			Value:     kv.Value,
			Version:   kv.Version,
			UpdatedAt: time.Now(),
		}
		cw.entries.Store(key, entry)
	}

	cw.logger.Info("Initial config loaded",
		StrField("prefix", cw.config.Prefix),
		IntField("count", len(resp.Kvs)))

	return nil
}

func (cw *ConfigWatcher) watchLoop(ctx context.Context) {
	for cw.running.Load() {
		watchCh := cw.etcdClient.Watch(ctx, cw.config.Prefix, clientv3.WithPrefix())

		for {
			select {
			case <-ctx.Done():
				return
			case watchResp, ok := <-watchCh:
				if !ok {
					if cw.running.Load() {
						cw.logger.Warn("Config watch channel closed, reconnecting...",
							StrField("prefix", cw.config.Prefix))
						time.Sleep(2 * time.Second)
					}
					goto RECONNECT
				}

				if err := watchResp.Err(); err != nil {
					cw.logger.Error("Config watch error",
						StrField("prefix", cw.config.Prefix),
						ErrField(err))
					goto RECONNECT
				}

				for _, event := range watchResp.Events {
					cw.handleEvent(event)
				}
			}
		}

	RECONNECT:
		if !cw.running.Load() {
			return
		}

		if err := cw.loadInitial(ctx); err != nil {
			cw.logger.Error("Failed to reload config after reconnect",
				StrField("prefix", cw.config.Prefix),
				ErrField(err))
			time.Sleep(cw.config.PollTick)
		}
	}
}

func (cw *ConfigWatcher) handleEvent(event *clientv3.Event) {
	key := string(event.Kv.Key)

	switch event.Type {
	case clientv3.EventTypePut:
		var oldValue []byte
		if existing, exists := cw.entries.Load(key); exists {
			oldValue = existing.Value
		}

		entry := &ConfigEntry{
			Key:       key,
			Value:     event.Kv.Value,
			Version:   event.Kv.Version,
			UpdatedAt: time.Now(),
		}
		cw.entries.Store(key, entry)

		cw.notifyCallbacks(key, oldValue, event.Kv.Value)

		cw.logger.Debug("Config updated",
			StrField("key", key),
			Int64Field("version", entry.Version))

	case clientv3.EventTypeDelete:
		var oldValue []byte
		if existing, exists := cw.entries.Load(key); exists {
			oldValue = existing.Value
		}

		cw.entries.Delete(key)

		cw.notifyCallbacks(key, oldValue, nil)

		cw.logger.Debug("Config deleted", StrField("key", key))
	}
}

func (cw *ConfigWatcher) notifyCallbacks(key string, oldValue, newValue []byte) {
	if callbacks, exists := cw.callbacks.Load(key); exists {
		for _, cb := range callbacks {
			func() {
				defer func() {
					if r := recover(); r != nil {
						cw.logger.Error("Config callback panic",
							StrField("key", key),
							AnyField("recover", r))
					}
				}()
				cb(key, oldValue, newValue)
			}()
		}
	}

	cw.callbacks.Range(func(k string, callbacks []ConfigChangeCallback) bool {
		if k == key {
			return true
		}
		if isPrefixMatch(k, key) {
			for _, cb := range callbacks {
				func() {
					defer func() {
						if r := recover(); r != nil {
							cw.logger.Error("Config prefix callback panic",
								StrField("pattern", k),
								StrField("key", key),
								AnyField("recover", r))
						}
					}()
					cb(key, oldValue, newValue)
				}()
			}
		}
		return true
	})
}

func isPrefixMatch(pattern, key string) bool {
	if len(pattern) == 0 {
		return true
	}
	pl := len(pattern)
	if pl > 0 && pattern[pl-1] == '*' {
		prefix := pattern[:pl-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	return pattern == key
}

func (cw *ConfigWatcher) OnChange(key string, callback ConfigChangeCallback) {
	callbacks, exists := cw.callbacks.Load(key)
	if !exists {
		callbacks = make([]ConfigChangeCallback, 0, 4)
	}
	callbacks = append(callbacks, callback)
	cw.callbacks.Store(key, callbacks)
}

func (cw *ConfigWatcher) OnPrefixChange(prefix string, callback ConfigChangeCallback) {
	pattern := prefix + "*"
	cw.OnChange(pattern, callback)
}

func (cw *ConfigWatcher) Get(key string) ([]byte, bool) {
	entry, exists := cw.entries.Load(key)
	if !exists {
		return nil, false
	}
	return entry.Value, true
}

func (cw *ConfigWatcher) GetJSON(key string, out interface{}) error {
	entry, exists := cw.entries.Load(key)
	if !exists {
		return fmt.Errorf("config key not found: %s", key)
	}
	return json.Unmarshal(entry.Value, out)
}

func (cw *ConfigWatcher) GetString(key string) (string, bool) {
	entry, exists := cw.entries.Load(key)
	if !exists {
		return "", false
	}
	return string(entry.Value), true
}

func (cw *ConfigWatcher) GetInt(key string) (int, bool) {
	entry, exists := cw.entries.Load(key)
	if !exists {
		return 0, false
	}
	var val int
	if err := json.Unmarshal(entry.Value, &val); err != nil {
		return 0, false
	}
	return val, true
}

func (cw *ConfigWatcher) GetAll() map[string]*ConfigEntry {
	result := make(map[string]*ConfigEntry)
	cw.entries.Range(func(key string, entry *ConfigEntry) bool {
		result[key] = entry
		return true
	})
	return result
}

func (cw *ConfigWatcher) EntryCount() int {
	return int(cw.entries.Len())
}

func (cw *ConfigWatcher) IsRunning() bool {
	return cw.running.Load()
}

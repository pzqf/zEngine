package zEvent

import (
	"sync"
	"sync/atomic"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

// EventHandler 事件处理器类型
type EventHandler func(event *Event)

// EventBus 事件总线
type EventBus struct {
	handlers map[EventType][]EventHandler // 事件处理器映射
	mu       sync.RWMutex                // 保护handlers的读写锁
	running  atomic.Bool                 // 事件总线运行状态
	logger   *zap.Logger                 // 日志记录器
}

// 全局事件总线实例
var globalEventBus *EventBus
var once sync.Once

// GetGlobalEventBus 获取全局事件总线实例（单例模式）
func GetGlobalEventBus() *EventBus {
	once.Do(func() {
		globalEventBus = NewEventBus()
	})
	return globalEventBus
}

// NewEventBus 创建新的事件总线
func NewEventBus() *EventBus {
	bus := &EventBus{
		handlers: make(map[EventType][]EventHandler),
		logger:   zLog.GetLogger(),
	}
	bus.running.Store(true)
	bus.logger.Info("EventBus initialized")
	return bus
}

// Subscribe 订阅特定类型的事件
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	if !eb.running.Load() {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// 如果该事件类型还没有处理器列表，创建一个
	if _, exists := eb.handlers[eventType]; !exists {
		eb.handlers[eventType] = make([]EventHandler, 0)
	}

	// 添加处理器
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	eb.logger.Debug("Subscribed to event",
		zap.Int("eventType", int(eventType)),
		zap.Int("handlerCount", len(eb.handlers[eventType])))
}

// Unsubscribe 取消订阅特定类型的事件（注意：由于Go语言限制，此方法目前不支持精确取消单个处理器）
func (eb *EventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	if !eb.running.Load() {
		return
	}

	eb.logger.Warn("Unsubscribe method is not fully implemented due to Go language limitations")
	// 注意：Go语言不允许直接比较函数值，因此无法精确查找并移除特定的处理器
	// 如需实现此功能，需要使用更复杂的设计，如为每个处理器分配唯一标识符
	// 目前可以考虑在不需要时清空整个事件类型的所有处理器
	// eb.mu.Lock()
	// defer eb.mu.Unlock()
	// delete(eb.handlers, eventType)
}

// Publish 发布事件（异步处理）
func (eb *EventBus) Publish(event *Event) {
	if !eb.running.Load() {
		return
	}

	eb.mu.RLock()
	handlers, exists := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		return
	}

	// 异步调用所有处理器
	for _, handler := range handlers {
		go func(h EventHandler, e *Event) {
			defer func() {
				if r := recover(); r != nil {
					eb.logger.Error("Panic in event handler",
						zap.Int("eventType", int(e.Type)),
						zap.Any("recover", r))
				}
			}()
			h(e)
		}(handler, event)
	}
}

// PublishSync 发布事件（同步处理，阻塞直到所有处理器完成）
func (eb *EventBus) PublishSync(event *Event) {
	if !eb.running.Load() {
		return
	}

	eb.mu.RLock()
	handlers, exists := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		return
	}

	// 同步调用所有处理器
	for _, handler := range handlers {
		defer func() {
			if r := recover(); r != nil {
				eb.logger.Error("Panic in event handler",
					zap.Int("eventType", int(event.Type)),
					zap.Any("recover", r))
			}
		}()
		handler(event)
	}
}

// Close 关闭事件总线
func (eb *EventBus) Close() {
	if !eb.running.CompareAndSwap(true, false) {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// 清空所有处理器
	for eventType := range eb.handlers {
		delete(eb.handlers, eventType)
	}

	eb.logger.Info("EventBus closed")
}

// GetHandlerCount 获取特定事件类型的处理器数量
func (eb *EventBus) GetHandlerCount(eventType EventType) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if handlers, exists := eb.handlers[eventType]; exists {
		return len(handlers)
	}
	return 0
}

// HasSubscribers 检查特定事件类型是否有订阅者
func (eb *EventBus) HasSubscribers(eventType EventType) bool {
	return eb.GetHandlerCount(eventType) > 0
}

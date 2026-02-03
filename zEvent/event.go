package zEvent

import "time"

// EventType 事件类型定义
type EventType int

// Event 事件基本结构
type Event struct {
	Type      EventType   // 事件类型
	Timestamp time.Time   // 事件发生时间
	Source    interface{} // 事件源
	Data      interface{} // 事件数据
}

// NewEvent 创建新事件
func NewEvent(eventType EventType, source interface{}, data interface{}) *Event {
	return &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    source,
		Data:      data,
	}
}

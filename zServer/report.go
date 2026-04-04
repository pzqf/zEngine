package zServer

import (
	"time"
)

// StateReport 状态报告
type StateReport struct {
	ServerID   string        `json:"serverId"`
	ServerType string        `json:"serverType"`
	State      ServerState   `json:"state"`
	Healthy    bool          `json:"healthy"`
	StartTime  time.Time     `json:"startTime"`
	Uptime     time.Duration `json:"uptime"`
}

// GetStateReport 获取状态报告
func (s *BaseServer) GetStateReport() StateReport {
	var serverID string
	if id := s.GetId(); id != nil {
		if idStr, ok := id.(string); ok {
			serverID = idStr
		}
	}
	return StateReport{
		ServerID:   serverID,
		ServerType: string(s.ServerType),
		State:      s.GetState(),
		Healthy:    s.IsStateActive(),
		StartTime:  s.startTime,
		Uptime:     time.Since(s.startTime),
	}
}

// GetStartTime 获取服务器启动时间
func (s *BaseServer) GetStartTime() time.Time {
	return s.startTime
}

// GetUptime 获取服务器运行时间
func (s *BaseServer) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

package zServer

import (
	"time"
)

// StateReport 状态报告
type StateReport struct {
	ServerID   string        `json:"serverId"`
	ServerType string        `json:"serverType"`
	State      ServerState   `json:"state"`
	Live       bool          `json:"live"`
	Ready      bool          `json:"ready"`
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
	state := s.GetState()
	live, ready, healthy := false, false, false
	if provider := s.getHealthProvider(); provider != nil {
		live, ready, healthy = provider.HealthStatus()
		live = live && !state.IsTerminal()
		ready = ready && state.IsActive()
		healthy = healthy && live && ready
	}
	return StateReport{
		ServerID:   serverID,
		ServerType: string(s.ServerType),
		State:      state,
		Live:       live,
		Ready:      ready,
		Healthy:    healthy,
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

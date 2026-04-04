package zServer

// RegisterComponent 注册组件
func (s *BaseServer) RegisterComponent(name string, component interface{}) {
	s.components.Store(name, component)
	if s.logger != nil {
		s.logger.Info("Component registered: %s", name)
	}
}

// GetComponent 获取组件
func (s *BaseServer) GetComponent(name string) interface{} {
	value, _ := s.components.Load(name)
	return value
}

// GetComponents 获取所有组件
func (s *BaseServer) GetComponents() map[string]interface{} {
	components := make(map[string]interface{})
	s.components.Range(func(key string, value interface{}) bool {
		components[key] = value
		return true
	})
	return components
}

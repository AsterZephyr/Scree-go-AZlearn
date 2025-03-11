package ws

// init 注册share事件处理器
// 在包初始化时被调用，将事件处理函数注册到事件处理系统中
func init() {
	register("share", func() Event {
		return &StartShare{}
	})
}

// StartShare 表示开始共享屏幕的事件
// 当用户点击"共享"按钮时触发
type StartShare struct{}

// Execute 处理开始共享事件
// 将用户标记为正在流式传输，并为每个其他用户创建WebRTC会话
func (e *StartShare) Execute(rooms *Rooms, current ClientInfo) error {
	// 获取当前用户所在的房间
	room, err := rooms.CurrentRoom(current)
	if err != nil {
		return err
	}

	// 将当前用户标记为正在流式传输
	room.Users[current.ID].Streaming = true

	// 获取TURN服务器的IPv4和IPv6地址
	v4, v6, err := rooms.config.TurnIPProvider.Get()
	if err != nil {
		return err
	}

	// 为房间中的每个其他用户创建WebRTC会话
	// 当前用户作为主机，其他用户作为客户端
	for _, user := range room.Users {
		if current.ID == user.ID {
			continue
		}
		room.newSession(current.ID, user.ID, rooms, v4, v6)
	}

	// 通知所有用户房间信息已更改
	room.notifyInfoChanged()
	return nil
}

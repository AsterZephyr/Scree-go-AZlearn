package ws

import (
	"fmt"
)

// init 注册join事件处理器
// 在包初始化时被调用，将加入房间事件处理函数注册到事件处理系统中
func init() {
	register("join", func() Event {
		return &Join{}
	})
}

// Join 表示用户加入房间的事件
// 包含要加入的房间ID和用户名信息
type Join struct {
	ID       string `json:"id"`       // 要加入的房间ID
	UserName string `json:"username,omitempty"` // 用户名，可选
}

// Execute 处理用户加入房间的逻辑
// 验证房间存在性，添加用户到房间，并设置相关连接
func (e *Join) Execute(rooms *Rooms, current ClientInfo) error {
	// 检查用户是否已经在某个房间中
	if rooms.connected[current.ID] != "" {
		return fmt.Errorf("cannot join room, you are already in one")
	}

	// 检查目标房间是否存在
	room, ok := rooms.Rooms[e.ID]
	if !ok {
		return fmt.Errorf("room with id %s does not exist", e.ID)
	}
	
	// 确定用户名
	name := e.UserName
	if current.Authenticated {
		// 如果用户已认证，使用认证用户名
		name = current.AuthenticatedUser
	}
	if name == "" {
		// 如果没有提供用户名，生成随机用户名
		name = rooms.RandUserName()
	}

	// 创建用户并添加到房间
	room.Users[current.ID] = &User{
		ID:        current.ID,
		Name:      name,
		Streaming: false,
		Owner:     false,
		Addr:      current.Addr,
		_write:    current.Write,
	}
	// 记录用户所在的房间
	rooms.connected[current.ID] = room.ID
	// 通知房间内所有用户信息已更改
	room.notifyInfoChanged()
	// 增加用户加入计数
	usersJoinedTotal.Inc()

	// 获取TURN服务器的IP地址
	v4, v6, err := rooms.config.TurnIPProvider.Get()
	if err != nil {
		return err
	}

	// 为房间中正在流式传输的用户创建新的会话
	// 这样新加入的用户可以看到已经在共享的屏幕
	for _, user := range room.Users {
		if current.ID == user.ID || !user.Streaming {
			continue
		}
		room.newSession(user.ID, current.ID, rooms, v4, v6)
	}

	return nil
}

package ws

import (
	"bytes"

	"github.com/AsterZephyr/Scree-go-AZlearn/ws/outgoing"
)

// init 注册stopshare事件处理器
// 在包初始化时被调用，将停止共享事件处理函数注册到事件处理系统中
func init() {
	register("stopshare", func() Event {
		return &StopShare{}
	})
}

// StopShare 表示停止屏幕共享的事件
// 这是一个不需要额外参数的事件类型
type StopShare struct{}

// Execute 处理停止屏幕共享的逻辑
// 更新用户状态，关闭相关会话，并通知其他用户
func (e *StopShare) Execute(rooms *Rooms, current ClientInfo) error {
	// 获取当前用户所在的房间
	room, err := rooms.CurrentRoom(current)
	if err != nil {
		return err
	}

	// 更新用户的共享状态为false
	room.Users[current.ID].Streaming = false
	
	// 遍历所有会话，关闭当前用户作为主机的会话
	for id, session := range room.Sessions {
		if bytes.Equal(session.Host.Bytes(), current.ID.Bytes()) {
			// 获取客户端用户
			client, ok := room.Users[session.Client]
			if ok {
				// 通知客户端共享已结束
				client.WriteTimeout(outgoing.EndShare(id))
			}
			// 关闭会话
			room.closeSession(rooms, id)
		}
	}

	// 通知房间内所有用户信息已更改
	room.notifyInfoChanged()
	return nil
}

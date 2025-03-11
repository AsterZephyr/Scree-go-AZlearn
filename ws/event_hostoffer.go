package ws

import (
	"fmt"

	"github.com/AsterZephyr/Scree-go-AZlearn/ws/outgoing"
	"github.com/rs/zerolog/log"
)

// init 注册hostoffer事件处理器
// 在包初始化时被调用，将事件处理函数注册到事件处理系统中
func init() {
	register("hostoffer", func() Event {
		return &HostOffer{}
	})
}

// HostOffer 表示主机发送的SDP offer消息
// 继承自outgoing.P2PMessage，包含会话ID和SDP信息
type HostOffer outgoing.P2PMessage

// Execute 处理主机发送的SDP offer
// 验证权限并将offer转发给对应的客户端
func (e *HostOffer) Execute(rooms *Rooms, current ClientInfo) error {
	// 获取当前用户所在的房间
	room, err := rooms.CurrentRoom(current)
	if err != nil {
		return err
	}

	// 查找对应的会话
	session, ok := room.Sessions[e.SID]

	if !ok {
		// 如果会话不存在，记录日志并忽略
		log.Debug().Str("id", e.SID.String()).Msg("unknown session")
		return nil
	}

	// 验证当前用户是否是会话的主机
	if session.Host != current.ID {
		return fmt.Errorf("permission denied for session %s", e.SID)
	}

	// 将offer转发给客户端
	room.Users[session.Client].WriteTimeout(outgoing.HostOffer(*e))

	return nil
}

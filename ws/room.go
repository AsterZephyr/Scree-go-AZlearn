package ws

import (
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/AsterZephyr/Scree-go-AZlearn/config"
	"github.com/AsterZephyr/Scree-go-AZlearn/ws/outgoing"
	"github.com/rs/xid"
	"github.com/rs/zerolog/log"
)

// ConnectionMode 定义了WebRTC连接的模式类型
type ConnectionMode string

const (
	// ConnectionLocal 表示仅使用本地连接（不使用STUN/TURN服务器）
	ConnectionLocal ConnectionMode = "local"
	// ConnectionSTUN 表示使用STUN服务器进行NAT穿透
	ConnectionSTUN ConnectionMode = "stun"
	// ConnectionTURN 表示使用TURN服务器进行媒体中继
	ConnectionTURN ConnectionMode = config.AuthModeTurn
)

// Room 表示一个共享房间，包含用户和会话信息
type Room struct {
	ID                string                  // 房间唯一标识符
	CloseOnOwnerLeave bool                    // 房主离开时是否关闭房间
	Mode              ConnectionMode          // 房间使用的连接模式
	Users             map[xid.ID]*User        // 房间中的用户映射
	Sessions          map[xid.ID]*RoomSession // 活跃的WebRTC会话映射
}

const (
	// CloseOwnerLeft 表示房间关闭的原因是房主离开
	CloseOwnerLeft = "Owner Left"
	// CloseDone 表示房间关闭的原因是正常结束
	CloseDone = "Read End"
)

// newSession 在房间中创建一个新的WebRTC会话
// 根据连接模式配置ICE服务器，并通知主机和客户端
func (r *Room) newSession(host, client xid.ID, rooms *Rooms, v4, v6 net.IP) {
	// 生成新的会话ID
	id := xid.New()
	// 创建会话并存储到映射中
	r.Sessions[id] = &RoomSession{
		Host:   host,
		Client: client,
	}
	sessionCreatedTotal.Inc()

	// 根据连接模式配置ICE服务器
	iceHost := []outgoing.ICEServer{}
	iceClient := []outgoing.ICEServer{}
	switch r.Mode {
	case ConnectionLocal:
		// 本地模式不需要ICE服务器
	case ConnectionSTUN:
		// STUN模式：配置STUN服务器地址
		iceHost = []outgoing.ICEServer{{URLs: rooms.addresses("stun", v4, v6, false)}}
		iceClient = []outgoing.ICEServer{{URLs: rooms.addresses("stun", v4, v6, false)}}
	case ConnectionTURN:
		// TURN模式：为主机和客户端生成TURN凭证
		hostName, hostPW := rooms.turnServer.Credentials(id.String()+"host", r.Users[host].Addr)
		clientName, clientPW := rooms.turnServer.Credentials(id.String()+"client", r.Users[client].Addr)
		iceHost = []outgoing.ICEServer{{
			URLs:       rooms.addresses("turn", v4, v6, true),
			Credential: hostPW,
			Username:   hostName,
		}}
		iceClient = []outgoing.ICEServer{{
			URLs:       rooms.addresses("turn", v4, v6, true),
			Credential: clientPW,
			Username:   clientName,
		}}
	}
	// 向主机和客户端发送会话信息
	r.Users[host].WriteTimeout(outgoing.HostSession{Peer: client, ID: id, ICEServers: iceHost})
	r.Users[client].WriteTimeout(outgoing.ClientSession{Peer: host, ID: id, ICEServers: iceClient})
}

// addresses 生成ICE服务器的URL地址列表
// 根据提供的IPv4和IPv6地址以及是否支持TCP生成不同的URL
func (r *Rooms) addresses(prefix string, v4, v6 net.IP, tcp bool) (result []string) {
	// 添加IPv4地址
	if v4 != nil {
		result = append(result, fmt.Sprintf("%s:%s:%s", prefix, v4.String(), r.config.TurnPort))
		if tcp {
			result = append(result, fmt.Sprintf("%s:%s:%s?transport=tcp", prefix, v4.String(), r.config.TurnPort))
		}
	}
	// 添加IPv6地址
	if v6 != nil {
		result = append(result, fmt.Sprintf("%s:[%s]:%s", prefix, v6.String(), r.config.TurnPort))
		if tcp {
			result = append(result, fmt.Sprintf("%s:[%s]:%s?transport=tcp", prefix, v6.String(), r.config.TurnPort))
		}
	}
	return
}

// closeSession 关闭指定的WebRTC会话
// 如果使用TURN模式，还会撤销TURN服务器的凭证
func (r *Room) closeSession(rooms *Rooms, id xid.ID) {
	if r.Mode == ConnectionTURN {
		// 撤销TURN服务器凭证
		rooms.turnServer.Disallow(id.String() + "host")
		rooms.turnServer.Disallow(id.String() + "client")
	}
	// 从映射中删除会话
	delete(r.Sessions, id)
	sessionClosedTotal.Inc()
}

// RoomSession 表示房间中的一个WebRTC会话
// 包含主机和客户端的ID
type RoomSession struct {
	Host   xid.ID // 主机（共享者）的ID
	Client xid.ID // 客户端（观看者）的ID
}

// notifyInfoChanged 通知房间中的所有用户房间信息已更改
// 发送更新后的用户列表给每个用户
func (r *Room) notifyInfoChanged() {
	for _, current := range r.Users {
		users := []outgoing.User{}
		// 构建用户列表
		for _, user := range r.Users {
			users = append(users, outgoing.User{
				ID:        user.ID,
				Name:      user.Name,
				Streaming: user.Streaming,
				You:       current == user, // 标记当前用户
				Owner:     user.Owner,      // 标记房主
			})
		}

		// 对用户列表进行排序：
		// 1. 房主优先
		// 2. 正在流式传输的用户优先
		// 3. 按名称字母顺序排序
		sort.Slice(users, func(i, j int) bool {
			left := users[i]
			right := users[j]

			if left.Owner != right.Owner {
				return left.Owner
			}

			if left.Streaming != right.Streaming {
				return left.Streaming
			}

			return left.Name < right.Name
		})

		// 发送房间信息给当前用户
		current.WriteTimeout(outgoing.Room{
			ID:    r.ID,
			Users: users,
		})
	}
}

// User 表示房间中的一个用户
type User struct {
	ID        xid.ID                  // 用户唯一标识符
	Addr      net.IP                  // 用户的IP地址
	Name      string                  // 用户名称
	Streaming bool                    // 是否正在共享屏幕
	Owner     bool                    // 是否是房主
	_write    chan<- outgoing.Message // 用于发送消息的通道
}

// WriteTimeout 向用户发送消息，带有超时处理
// 如果2秒内无法发送，则记录警告日志
func (u *User) WriteTimeout(msg outgoing.Message) {
	writeTimeout(u._write, msg)
}

// writeTimeout 是一个泛型函数，用于向通道发送消息，带有超时处理
// 如果2秒内无法发送，则记录警告日志
func writeTimeout[T any](ch chan<- T, msg T) {
	select {
	case <-time.After(2 * time.Second):
		log.Warn().Interface("event", fmt.Sprintf("%T", msg)).Interface("payload", msg).Msg("Client write loop didn't accept the message.")
	case ch <- msg:
	}
}

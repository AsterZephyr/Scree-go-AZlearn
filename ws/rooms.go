package ws

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/AsterZephyr/Scree-go-AZlearn/auth"
	"github.com/AsterZephyr/Scree-go-AZlearn/config"
	"github.com/AsterZephyr/Scree-go-AZlearn/turn"
	"github.com/AsterZephyr/Scree-go-AZlearn/util"
	"github.com/gorilla/websocket"
	"github.com/rs/xid"
	"github.com/rs/zerolog/log"
)

// NewRooms 创建一个新的Rooms实例
// 初始化所有必要的字段并返回准备好的Rooms对象
// 参数:
// - tServer: TURN服务器实例，用于WebRTC连接
// - users: 用户认证管理器
// - conf: 应用配置
func NewRooms(tServer turn.Server, users *auth.Users, conf config.Config) *Rooms {
	return &Rooms{
		Rooms:      map[string]*Room{},          // 初始化空房间映射
		Incoming:   make(chan ClientMessage),    // 创建消息通道
		connected:  map[xid.ID]string{},         // 初始化客户端连接映射
		turnServer: tServer,                     // 设置TURN服务器
		users:      users,                       // 设置用户管理器
		config:     conf,                        // 设置配置
		r:          rand.New(rand.NewSource(time.Now().Unix())), // 初始化随机数生成器
		upgrader: websocket.Upgrader{            // 配置WebSocket升级器
			ReadBufferSize:  1024,               // 读缓冲区大小
			WriteBufferSize: 1024,               // 写缓冲区大小
			CheckOrigin: func(r *http.Request) bool { // 跨域检查函数
				origin := r.Header.Get("origin")
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				if u.Host == r.Host {
					return true
				}
				return conf.CheckOrigin(origin)
			},
		},
	}
}

// Rooms 管理所有房间和WebSocket连接
// 处理客户端消息、房间创建和删除、用户加入和离开等操作
type Rooms struct {
	turnServer turn.Server             // TURN服务器，用于WebRTC连接
	Rooms      map[string]*Room        // 所有活跃房间的映射，键为房间ID
	Incoming   chan ClientMessage      // 接收客户端消息的通道
	upgrader   websocket.Upgrader      // WebSocket连接升级器
	users      *auth.Users             // 用户认证管理器
	config     config.Config           // 应用配置
	r          *rand.Rand              // 随机数生成器，用于生成随机名称
	connected  map[xid.ID]string       // 客户端ID到房间ID的映射，记录每个客户端所在的房间
}

// CurrentRoom 获取客户端当前所在的房间
// 根据客户端信息查找对应的房间并返回
// 参数:
// - info: 客户端信息
// 返回:
// - 房间指针和可能的错误
func (r *Rooms) CurrentRoom(info ClientInfo) (*Room, error) {
	// 查找客户端是否已连接
	roomID, ok := r.connected[info.ID]
	if !ok {
		return nil, fmt.Errorf("not connected")
	}
	// 检查客户端是否在房间中
	if roomID == "" {
		return nil, fmt.Errorf("not in a room")
	}
	// 查找房间是否存在
	room, ok := r.Rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("room with id %s does not exist", roomID)
	}

	return room, nil
}

// RandUserName 生成一个随机的用户名
// 使用util包中的函数生成随机名称
func (r *Rooms) RandUserName() string {
	return util.NewUserName(r.r)
}

// RandRoomName 生成一个随机的房间名
// 使用util包中的函数生成随机名称
func (r *Rooms) RandRoomName() string {
	return util.NewRoomName(r.r)
}

// Upgrade 将HTTP连接升级为WebSocket连接
// 处理WebSocket握手并创建新的客户端连接
// 参数:
// - w: HTTP响应写入器
// - req: HTTP请求
func (r *Rooms) Upgrade(w http.ResponseWriter, req *http.Request) {
	// 将HTTP连接升级为WebSocket连接
	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Debug().Err(err).Msg("Websocket upgrade")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(fmt.Sprintf("Upgrade failed %s", err)))
		return
	}

	// 获取当前用户信息
	user, loggedIn := r.users.CurrentUser(req)
	// 创建新的客户端
	c := newClient(conn, req, r.Incoming, user, loggedIn, r.config.TrustProxyHeaders)
	// 发送连接事件
	r.Incoming <- ClientMessage{Info: c.info, Incoming: Connected{}, SkipConnectedCheck: true}

	// 启动读取和写入处理
	go c.startReading(time.Second * 20)
	go c.startWriteHandler(time.Second * 5)
}

// Start 启动房间管理器的主循环
// 处理来自客户端的所有消息
func (r *Rooms) Start() {
	for msg := range r.Incoming {
		// 检查客户端是否已连接
		_, connected := r.connected[msg.Info.ID]
		if !msg.SkipConnectedCheck && !connected {
			log.Debug().Interface("event", fmt.Sprintf("%T", msg.Incoming)).Interface("payload", msg.Incoming).Msg("WebSocket Ignore")
			continue
		}

		// 执行事件处理
		if err := msg.Incoming.Execute(r, msg.Info); err != nil {
			// 如果处理出错，断开客户端连接
			dis := Disconnected{Code: websocket.CloseNormalClosure, Reason: err.Error()}
			dis.executeNoError(r, msg.Info)
		}
	}
}

// Count 获取当前房间数量
// 通过健康检查事件获取房间数量，带有超时处理
// 返回:
// - 房间数量和可能的错误消息
func (r *Rooms) Count() (int, string) {
	timeout := time.After(5 * time.Second)

	// 创建健康检查事件
	h := Health{Response: make(chan int, 1)}
	select {
	case r.Incoming <- ClientMessage{SkipConnectedCheck: true, Incoming: &h}:
	case <-timeout:
		return -1, "main loop didn't accept a message within 5 second"
	}
	select {
	case count := <-h.Response:
		return count, ""
	case <-timeout:
		return -1, "main loop didn't respond to a message within 5 second"
	}
}

// closeRoom 关闭并删除一个房间
// 清理房间中的所有会话和用户连接
// 参数:
// - roomID: 要关闭的房间ID
func (r *Rooms) closeRoom(roomID string) {
	room, ok := r.Rooms[roomID]
	if !ok {
		return
	}
	// 更新用户离开计数
	usersLeftTotal.Add(float64(len(room.Users)))
	// 关闭房间中的所有会话
	for id := range room.Sessions {
		room.closeSession(r, id)
	}

	// 从房间映射中删除房间
	delete(r.Rooms, roomID)
	// 更新房间关闭计数
	roomsClosedTotal.Inc()
}

package ws

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/AsterZephyr/Scree-go-AZlearn/ws/outgoing"
)

// ping 向WebSocket连接发送ping消息
// 用于检测连接是否仍然活跃
var ping = func(conn *websocket.Conn) error {
	return conn.WriteMessage(websocket.PingMessage, nil)
}

// writeJSON 向WebSocket连接写入JSON消息
var writeJSON = func(conn *websocket.Conn, v interface{}) error {
	return conn.WriteJSON(v)
}

const (
	// writeWait 定义了写操作的超时时间
	writeWait = 2 * time.Second
)

// Client 表示一个WebSocket客户端连接
type Client struct {
	conn *websocket.Conn    // WebSocket连接
	info ClientInfo         // 客户端信息
	once once               // 确保关闭操作只执行一次
	read chan<- ClientMessage // 读取到的消息发送到此通道
}

// ClientMessage 表示从客户端接收到的消息
type ClientMessage struct {
	Info               ClientInfo // 客户端信息
	SkipConnectedCheck bool       // 是否跳过连接检查
	Incoming           Event      // 接收到的事件
}

// ClientInfo 包含客户端的基本信息
type ClientInfo struct {
	ID                xid.ID             // 客户端唯一标识符
	Authenticated     bool               // 是否已认证
	AuthenticatedUser string             // 认证用户名
	Write             chan outgoing.Message // 发送消息的通道
	Addr              net.IP             // 客户端IP地址
}

// newClient 创建一个新的WebSocket客户端
// 初始化客户端信息并返回客户端实例
func newClient(conn *websocket.Conn, req *http.Request, read chan ClientMessage, authenticatedUser string, authenticated, trustProxy bool) *Client {
	// 获取客户端IP地址
	ip := conn.RemoteAddr().(*net.TCPAddr).IP
	// 如果配置了信任代理，则尝试从X-Real-IP头获取真实IP
	if realIP := req.Header.Get("X-Real-IP"); trustProxy && realIP != "" {
		ip = net.ParseIP(realIP)
	}

	// 创建客户端实例
	client := &Client{
		conn: conn,
		info: ClientInfo{
			Authenticated:     authenticated,
			AuthenticatedUser: authenticatedUser,
			ID:                xid.New(),
			Addr:              ip,
			Write:             make(chan outgoing.Message, 1),
		},
		read: read,
	}
	client.debug().Msg("WebSocket New Connection")
	return client
}

// CloseOnError 在发生错误时关闭连接
// 发送断开连接事件并关闭WebSocket连接
func (c *Client) CloseOnError(code int, reason string) {
	c.once.Do(func() {
		// 发送断开连接事件
		go func() {
			c.read <- ClientMessage{
				Info: c.info,
				Incoming: &Disconnected{
					Code:   code,
					Reason: reason,
				},
			}
		}()
		// 关闭WebSocket连接
		c.writeCloseMessage(code, reason)
	})
}

// CloseOnDone 在正常完成时关闭连接
// 只关闭WebSocket连接，不发送断开连接事件
func (c *Client) CloseOnDone(code int, reason string) {
	c.once.Do(func() {
		c.writeCloseMessage(code, reason)
	})
}

// writeCloseMessage 向客户端发送关闭消息并关闭连接
func (c *Client) writeCloseMessage(code int, reason string) {
	message := websocket.FormatCloseMessage(code, reason)
	_ = c.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(writeWait))
	c.conn.Close()
}

// startReading 开始从客户端读取消息
// 处理接收到的消息并在出错时关闭连接
func (c *Client) startReading(pongWait time.Duration) {
	defer c.CloseOnError(websocket.CloseNormalClosure, "Reader Routine Closed")

	// 设置读取超时和pong处理函数
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(appData string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	
	// 持续读取消息
	for {
		t, m, err := c.conn.NextReader()
		if err != nil {
			c.CloseOnError(websocket.CloseNormalClosure, "read error: "+err.Error())
			return
		}
		// 不支持二进制消息
		if t == websocket.BinaryMessage {
			c.CloseOnError(websocket.CloseUnsupportedData, "unsupported binary message type")
			return
		}

		// 解析接收到的消息
		incoming, err := ReadTypedIncoming(m)
		if err != nil {
			c.CloseOnError(websocket.CloseUnsupportedData, fmt.Sprintf("malformed message: %s", err))
			return
		}
		c.debug().Interface("event", fmt.Sprintf("%T", incoming)).Interface("payload", incoming).Msg("WebSocket Receive")
		// 将消息发送到读取通道
		c.read <- ClientMessage{Info: c.info, Incoming: incoming}
	}
}

// startWriteHandler 开始向客户端写入消息
// 处理发送消息、定期ping和错误处理
func (c *Client) startWriteHandler(pingPeriod time.Duration) {
	// 创建定期ping的定时器
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()
	defer func() {
		c.debug().Msg("WebSocket Done")
	}()
	defer c.conn.Close()
	
	// 持续处理写入操作
	for {
		select {
		case message := <-c.info.Write:
			// 处理关闭消息
			if msg, ok := message.(outgoing.CloseWriter); ok {
				c.debug().Str("reason", msg.Reason).Int("code", msg.Code).Msg("WebSocket Close")
				c.CloseOnDone(msg.Code, msg.Reason)
				return
			}

			// 设置写入超时
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			// 将消息转换为类型化消息
			typed, err := ToTypedOutgoing(message)
			c.debug().Interface("event", typed.Type).Interface("payload", typed.Payload).Msg("WebSocket Send")
			if err != nil {
				c.debug().Err(err).Msg("could not get typed message, exiting connection.")
				c.CloseOnError(websocket.CloseNormalClosure, "malformed outgoing "+err.Error())
				continue
			}

			// 写入JSON消息
			if err := writeJSON(c.conn, typed); err != nil {
				c.printWebSocketError("write", err)
				c.CloseOnError(websocket.CloseNormalClosure, "write error"+err.Error())
			}
		case <-pingTicker.C:
			// 定期发送ping消息
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ping(c.conn); err != nil {
				c.printWebSocketError("ping", err)
				c.CloseOnError(websocket.CloseNormalClosure, "ping timeout")
			}
		}
	}
}

// debug 返回一个带有客户端信息的日志事件
// 用于记录与客户端相关的调试信息
func (c *Client) debug() *zerolog.Event {
	return log.Debug().Str("id", c.info.ID.String()).Str("ip", c.info.Addr.String())
}

// printWebSocketError 打印WebSocket错误
// 过滤掉一些常见的正常关闭错误
func (c *Client) printWebSocketError(typex string, err error) {
	// 忽略已关闭连接的错误
	if strings.Contains(err.Error(), "use of closed network connection") {
		return
	}
	closeError, ok := err.(*websocket.CloseError)

	// 忽略正常关闭的错误
	if ok && closeError != nil && (closeError.Code == 1000 || closeError.Code == 1001) {
		// normal closure
		return
	}

	// 记录其他错误
	c.debug().Str("type", typex).Err(err).Msg("WebSocket Error")
}

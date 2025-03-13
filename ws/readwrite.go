package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/AsterZephyr/Scree-go-AZlearn/ws/outgoing"
)

// Typed 表示一个带类型的WebSocket消息
// 用于在JSON序列化和反序列化过程中保留消息类型信息
type Typed struct {
	Type    string          `json:"type"`    // 消息类型，用于标识不同种类的消息
	Payload json.RawMessage `json:"payload"` // 消息内容，使用原始JSON格式存储
}

// ToTypedOutgoing 将outgoing包中的消息转换为带类型的WebSocket消息
// 这个函数用于准备发送到客户端的消息
// 参数outgoing是要发送的消息对象
// 返回转换后的Typed对象和可能的错误
func ToTypedOutgoing(outgoing outgoing.Message) (Typed, error) {
	// 将消息对象序列化为JSON
	payload, err := json.Marshal(outgoing)
	if err != nil {
		return Typed{}, err
	}
	// 创建并返回带类型的消息
	return Typed{
		Type:    outgoing.Type(), // 获取消息类型
		Payload: payload,         // 使用序列化后的JSON作为载荷
	}, nil
}

// ReadTypedIncoming 从读取器中解析带类型的WebSocket消息
// 并创建对应的事件对象
// 参数r是包含JSON消息的读取器
// 返回解析后的事件对象和可能的错误
func ReadTypedIncoming(r io.Reader) (Event, error) {
	typed := Typed{}
	// 从读取器解码JSON到Typed结构体
	if err := json.NewDecoder(r).Decode(&typed); err != nil {
		return nil, fmt.Errorf("%s e", err)
	}

	// 查找消息类型对应的事件创建函数
	create, ok := provider[typed.Type]

	if !ok {
		return nil, errors.New("cannot handle " + typed.Type)
	}

	// 创建对应类型的事件对象
	payload := create()

	// 将JSON载荷解码到事件对象
	if err := json.Unmarshal(typed.Payload, payload); err != nil {
		return nil, fmt.Errorf("incoming payload %s", err)
	}
	return payload, nil
}

// provider 存储所有已注册的事件类型和对应的创建函数
// 键是事件类型字符串，值是创建对应事件对象的函数
var provider = map[string]func() Event{}

// register 注册一个事件类型和对应的创建函数
// 这个函数在各个事件类型的init函数中被调用
// 参数t是事件类型字符串
// 参数incoming是创建事件对象的函数
func register(t string, incoming func() Event) {
	provider[t] = incoming
}

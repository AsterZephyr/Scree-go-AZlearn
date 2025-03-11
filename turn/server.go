package turn

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/turn/v4"
	"github.com/rs/zerolog/log"
	"github.com/AsterZephyr/Scree-go-AZlearn/config"
	"github.com/AsterZephyr/Scree-go-AZlearn/config/ipdns"
	"github.com/AsterZephyr/Scree-go-AZlearn/util"
)

// Server 定义了TURN服务器的接口
// 提供了凭证生成和撤销的功能
type Server interface {
	// Credentials 为指定ID和IP地址生成TURN服务器的用户名和密码
	Credentials(id string, addr net.IP) (string, string)
	// Disallow 撤销指定用户名的访问权限
	Disallow(username string)
}

// InternalServer 实现了内部TURN服务器
// 直接在Screego服务器内部运行TURN服务
type InternalServer struct {
	lock   sync.RWMutex     // 用于保护lookup映射的读写锁
	lookup map[string]Entry // 存储用户名到凭证条目的映射
}

// ExternalServer 实现了外部TURN服务器连接
// 用于连接到外部运行的TURN服务
type ExternalServer struct {
	secret []byte        // 用于生成HMAC的密钥
	ttl    time.Duration // 凭证的有效期
}

// Entry 表示TURN服务器中的一个用户条目
type Entry struct {
	addr     net.IP // 用户的IP地址
	password []byte // 用户的密码（已经过哈希处理）
}

// Realm 定义了TURN服务器的域
const Realm = "screego"

// Generator 是自定义的中继地址生成器
// 扩展了turn库的RelayAddressGenerator接口
type Generator struct {
	turn.RelayAddressGenerator
	IPProvider ipdns.Provider // 提供IP地址的服务
}

// AllocatePacketConn 分配一个网络连接和地址用于TURN中继
// 重写了基础实现以使用配置的IP地址
func (r *Generator) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	// 首先调用基础实现分配连接
	conn, addr, err := r.RelayAddressGenerator.AllocatePacketConn(network, requestedPort)
	if err != nil {
		return conn, addr, err
	}
	relayAddr := *addr.(*net.UDPAddr)

	// 获取配置的IPv4和IPv6地址
	v4, v6, err := r.IPProvider.Get()
	if err != nil {
		return conn, addr, err
	}

	// 根据网络情况选择合适的IP地址
	if v6 == nil || (relayAddr.IP.To4() != nil && v4 != nil) {
		relayAddr.IP = v4
	} else {
		relayAddr.IP = v6
	}
	if err == nil {
		log.Debug().Str("addr", addr.String()).Str("relayaddr", relayAddr.String()).Msg("TURN allocated")
	}
	return conn, &relayAddr, err
}

// Start 根据配置启动TURN服务器
// 根据配置决定使用内部还是外部TURN服务器
func Start(conf config.Config) (Server, error) {
	if conf.TurnExternal {
		return newExternalServer(conf)
	} else {
		return newInternalServer(conf)
	}
}

// newExternalServer 创建一个外部TURN服务器连接
// 使用配置的密钥和24小时的TTL
func newExternalServer(conf config.Config) (Server, error) {
	return &ExternalServer{
		secret: []byte(conf.TurnExternalSecret),
		ttl:    24 * time.Hour,
	}, nil
}

// newInternalServer 创建并启动一个内部TURN服务器
// 设置UDP和TCP监听器，配置权限和认证
func newInternalServer(conf config.Config) (Server, error) {
	// 创建UDP监听器
	udpListener, err := net.ListenPacket("udp", conf.TurnAddress)
	if err != nil {
		return nil, fmt.Errorf("udp: could not listen on %s: %s", conf.TurnAddress, err)
	}
	// 创建TCP监听器
	tcpListener, err := net.Listen("tcp", conf.TurnAddress)
	if err != nil {
		return nil, fmt.Errorf("tcp: could not listen on %s: %s", conf.TurnAddress, err)
	}

	// 创建服务器实例
	svr := &InternalServer{lookup: map[string]Entry{}}

	// 创建中继地址生成器
	gen := &Generator{
		RelayAddressGenerator: generator(conf),
		IPProvider:            conf.TurnIPProvider,
	}

	// 定义权限处理函数，用于控制哪些对等方可以连接
	var permissions turn.PermissionHandler = func(clientAddr net.Addr, peerIP net.IP) bool {
		// 检查是否在拒绝列表中
		for _, cidr := range conf.TurnDenyPeersParsed {
			if cidr.Contains(peerIP) {
				return false
			}
		}

		return true
	}

	// 创建并启动TURN服务器
	_, err = turn.NewServer(turn.ServerConfig{
		Realm:       Realm,
		AuthHandler: svr.authenticate, // 设置认证处理函数
		ListenerConfigs: []turn.ListenerConfig{
			{Listener: tcpListener, RelayAddressGenerator: gen, PermissionHandler: permissions},
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{PacketConn: udpListener, RelayAddressGenerator: gen, PermissionHandler: permissions},
		},
	})
	if err != nil {
		return nil, err
	}

	log.Info().Str("addr", conf.TurnAddress).Msg("Start TURN/STUN")
	return svr, nil
}

// generator 根据配置创建合适的中继地址生成器
// 如果配置了端口范围，则使用端口范围生成器
func generator(conf config.Config) turn.RelayAddressGenerator {
	min, max, useRange := conf.PortRange()
	if useRange {
		log.Debug().Uint16("min", min).Uint16("max", max).Msg("Using Port Range")
		return &RelayAddressGeneratorPortRange{MinPort: min, MaxPort: max}
	}
	return &RelayAddressGeneratorNone{}
}

// allow 为指定用户名和密码添加访问权限
// 生成认证密钥并存储到lookup映射中
func (a *InternalServer) allow(username, password string, addr net.IP) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.lookup[username] = Entry{
		addr:     addr,
		password: turn.GenerateAuthKey(username, Realm, password),
	}
}

// Disallow 实现Server接口，撤销指定用户名的访问权限
// 从lookup映射中删除用户条目
func (a *InternalServer) Disallow(username string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	delete(a.lookup, username)
}

// Disallow 实现Server接口，对于外部服务器不支持直接撤销
// 外部服务器的凭证会在TTL到期后自动失效
func (a *ExternalServer) Disallow(username string) {
	// 不支持，将在TTL到期后自动失效
}

// authenticate 是TURN服务器的认证回调函数
// 检查用户名是否存在并返回对应的密码
func (a *InternalServer) authenticate(username, realm string, addr net.Addr) ([]byte, bool) {
	a.lock.RLock()
	defer a.lock.RUnlock()

	entry, ok := a.lookup[username]

	if !ok {
		log.Debug().Interface("addr", addr).Str("username", username).Msg("TURN username not found")
		return nil, false
	}

	log.Debug().Interface("addr", addr.String()).Str("realm", realm).Msg("TURN authenticated")
	return entry.password, true
}

// Credentials 实现Server接口，为内部服务器生成凭证
// 生成随机密码并调用allow方法添加权限
func (a *InternalServer) Credentials(id string, addr net.IP) (string, string) {
	password := util.RandString(20)
	a.allow(id, password, addr)
	return id, password
}

// Credentials 实现Server接口，为外部服务器生成凭证
// 使用HMAC-SHA1生成基于时间的临时凭证
func (a *ExternalServer) Credentials(id string, addr net.IP) (string, string) {
	// 用户名格式：过期时间戳:ID
	username := fmt.Sprintf("%d:%s", time.Now().Add(a.ttl).Unix(), id)
	// 使用HMAC-SHA1生成密码
	mac := hmac.New(sha1.New, a.secret)
	_, _ = mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return username, password
}

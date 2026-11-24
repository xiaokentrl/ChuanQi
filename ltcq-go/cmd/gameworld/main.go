// Command gameworld 是 Go 重写的 gameworld 服务端骨架（里程碑 M4/M5）。
// 监听 6001 接受网关连接，解析网关帧并驱动 Lua 业务层（OnNetMsg 等）。
// 当前为骨架：登录帧（未绑定 actor）先建会话，业务帧路由到 OnNetMsg。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/engineapi"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

func main() {
	addr := flag.String("listen", "0.0.0.0:6001", "网关连接监听地址")
	dataRoot := flag.String("data", "", "gameworld/data 目录路径")
	ostype := flag.String("ostype", "WIN", "OSTYPE（条件编译用）")
	dsn := flag.String("dsn", "", "MySQL DSN（引擎级登录校验；空则不启用登录，如 root:123456@tcp(127.0.0.1:3306)/actor_s1?charset=utf8）")
	flag.Parse()

	if *dataRoot == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -data <gameworld/data 目录>")
		os.Exit(2)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		slog.Error("监听失败", "addr", *addr, "err", err)
		os.Exit(1)
	}
	slog.Info("gameworld(go) 监听", "addr", *addr, "data", *dataRoot)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept 失败", "err", err)
			continue
		}
		go handleGate(conn, *dataRoot, *ostype, *dsn)
	}
}

// handleGate 处理一个网关连接：读注册握手 → 帧 pump → 路由。
func handleGate(conn net.Conn, dataRoot, ostype, dsn string) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	// 1. 读 80B 网关注册包
	handshake := make([]byte, netframe.HandshakeSize)
	if _, err := readFull(conn, handshake); err != nil {
		slog.Error("读网关注册失败", "remote", remote, "err", err)
		return
	}
	h, err := netframe.ParseHandshake(handshake)
	if err != nil {
		slog.Error("网关注册解析失败", "remote", remote, "err", err)
		return
	}
	slog.Info("网关注册成功", "remote", remote, "name", h.Name, "serverId", h.ServerID)

	// 2. 组装引擎（每个网关连接一个 VM；单网关场景适用）
	var writeMu sync.Mutex
	var engine *engineapi.Engine
	sender := func(frame []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_, err := conn.Write(frame)
		return err
	}
	engine, err = engineapi.BootstrapWithLogin(dataRoot, ostype, dsn, sender)
	if err != nil {
		slog.Error("引擎初始化失败", "err", err)
		return
	}
	defer engine.Close()

	// 3. 帧 pump
	r := bufio.NewReaderSize(conn, 1<<20)
	for {
		frame, err := netframe.ReadGateFrame(r)
		if err != nil {
			slog.Info("网关连接结束", "remote", remote, "err", err)
			return
		}
		handleFrame(frame, engine)
	}
}

// handleFrame 分派单帧。
func handleFrame(f netframe.GateFrame, engine *engineapi.Engine) {
	switch f.Cmd {
	case netframe.GateCmdHeartbeat:
		// 心跳：无需回应（对齐 C++ 网关行为）
	case netframe.GateCmdUserOpen:
		// cmd=1 UserOpen：NetId(8B) + ...；提取 sid 建会话
		if len(f.Body) >= netframe.NetIDLen {
			sid := netframe.NetIDSid(f.Body[:netframe.NetIDLen])
			engine.Router.OpenSession(sid, "")
		}
	case netframe.GateCmdUserClose:
		// cmd=2 UserClose：清理会话
		if len(f.Body) >= netframe.NetIDLen {
			sid := netframe.NetIDSid(f.Body[:netframe.NetIDLen])
			engine.Router.CloseSession(sid)
		}
	case netframe.GateCmdUserData:
		// cmd=5 UserData：上行路由到 OnNetMsg
		// 诊断日志：NetId 之后的 [sysid][cmdid] 决定 Lua 分发目标（登录帧 sysid=0x01/cmdid=0x00）。
		if len(f.Body) >= netframe.NetIDLen+2 {
			sid := netframe.NetIDSid(f.Body[:netframe.NetIDLen])
			slog.Info("收到上行数据帧", "sid", sid, "sysid", f.Body[netframe.NetIDLen], "cmdid", f.Body[netframe.NetIDLen+1], "len", len(f.Body))
		}
		if err := engine.Router.RouteUpstream(f.Body); err != nil {
			slog.Error("上行路由失败", "err", err)
		}
	default:
		slog.Info("未处理的网关帧", "cmd", f.Cmd, "len", len(f.Body))
	}
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

package engineapi

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// registries.go：五大回调注册表（ROADMAP 4.4）。
//
// 策略：加载原版 5 个 Lua module（Lua 零改动），Go 侧注册它们依赖的 System stub，
// 触发时调用 Lua 挂到 _G 的 On* 入口。csmsg 按 ADR-005 首版 stub（注册成功但不转发）。

// RegistryModule 是一个原版注册表模块的描述。
type RegistryModule struct {
	Name    string
	RelPath string   // 相对 data 根
	Globals []string // 需要暴露的全局回调名（用于校验加载成功）
}

// RegistryModules 五大注册表模块（路径已从原版资产核实）。
var RegistryModules = []RegistryModule{
	{"netmsgdispatcher", "functions/utils/net/netmsgdispatcher.lua", []string{"OnNetMsg"}},
	{"dbretdispatcher", "functions/utils/net/dbretdispatcher.lua", []string{"OnDbRetDispatcher"}},
	{"actorevent", "functions/actorevent/actorevent.lua", []string{"OnActorEvent"}},
	{"csmsgdispatcher", "functions/crossserver/csmsgdispatcher.lua", []string{"OnRecvCrossServerMsg"}},
	{"monevent", "functions/monevent/monevent.lua", []string{"OnMonsterKilled", "OnMonsterAttacked", "OnMonsterCreate"}},
}

// InstallSystemStubs 注册五大注册表 module 加载时依赖的 System.* Go stub。
// 这些 API 由 C++ 引擎提供，签名对齐原版调用点。
func InstallSystemStubs(L luahost.Interpreter) {
	noRet := func(a ...luahost.Value) (luahost.Value, error) { return nil, nil }
	// netmsgdispatcher.lua:18
	L.Register("System.regScriptNetMsg", noRet)
	// dbretdispatcher.lua:27
	L.Register("System.setDbRetRoute", noRet)
	// monevent.lua 底层挂钩（L56/64/72/88/104/112）
	for _, m := range []string{
		"System.regMonsterDie", "System.regMonsterCreate", "System.regMonsterAttacked",
		"System.regMonsterHpChange", "System.regMonsterDamage", "System.regGatherFinish",
	} {
		L.Register(m, noRet)
	}
	// 通用 System API
	L.Register("System.getServerId", func(a ...luahost.Value) (luahost.Value, error) { return 1, nil })
	L.Register("System.getGlobalNpc", func(a ...luahost.Value) (luahost.Value, error) { return 0, nil })
	L.Register("System.getNowTime", realGetNowTime)
	L.RegisterMulti("System.timeDecode", realTimeDecode)
	L.Register("System.isSameDay", realIsSameDay)
	staticVar, staticErr := L.DoString("{}")
	L.Register("System.getStaticVar", func(a ...luahost.Value) (luahost.Value, error) { return staticVar, staticErr })
	L.Register("System.logCounter", noRet)
	L.Register("System.SendToDb", noRet)
	L.Register("System.allocDBPacket", func(a ...luahost.Value) (luahost.Value, error) { return 0, nil })
	L.Register("System.flushDBPacket", noRet)
}

func realGetNowTime(a ...luahost.Value) (luahost.Value, error) {
	return time.Now().Unix(), nil
}

func realTimeDecode(a ...luahost.Value) ([]luahost.Value, error) {
	if len(a) == 0 {
		return []luahost.Value{0, 0, 0, 0, 0, 0}, nil
	}
	timestamp, err := integerArgument(a[0])
	if err != nil {
		return []luahost.Value{0, 0, 0, 0, 0, 0}, nil
	}
	t := time.Unix(int64(timestamp), 0).In(time.Local)
	return []luahost.Value{t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()}, nil
}

func realIsSameDay(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return false, nil
	}
	left, err := integerArgument(a[0])
	if err != nil {
		return false, nil
	}
	right, err := integerArgument(a[1])
	if err != nil {
		return false, nil
	}
	leftTime := time.Unix(int64(left), 0).In(time.Local)
	rightTime := time.Unix(int64(right), 0).In(time.Local)
	return leftTime.Year() == rightTime.Year() && leftTime.YearDay() == rightTime.YearDay(), nil
}

// LoadRegistries 加载五大注册表 Lua module 并返回加载结果。
func LoadRegistries(L luahost.Interpreter, dataRoot string) ([]string, error) {
	var loaded []string
	for _, m := range RegistryModules {
		path := filepath.Join(dataRoot, filepath.FromSlash(m.RelPath))
		if err := L.LoadFile(path); err != nil {
			return loaded, fmt.Errorf("加载注册表 %s: %w", m.Name, err)
		}
		loaded = append(loaded, m.Name)
	}
	return loaded, nil
}

// Fire 触发一个全局回调（如 OnNetMsg/OnActorEvent）。args 顺序对齐原版回调签名。
func Fire(L luahost.Interpreter, global string, args ...luahost.Value) (luahost.Value, error) {
	return L.Call(global, args...)
}

// FirePCall 带超时的触发（防止业务回调死循环卡死引擎）。
func FirePCall(L luahost.Interpreter, timeoutMs int, global string, args ...luahost.Value) (luahost.Value, error) {
	return L.PCall(time.Duration(timeoutMs)*time.Millisecond, global, args...)
}

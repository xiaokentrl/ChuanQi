package luahost

import (
	"fmt"
	"path/filepath"
)

// InjectEnums 加载原版 _G.lua 的 1214 个常量并补注 C++ 注册的全局表。
//
// _G.lua 是纯 `_G["name"] = value` 序列，直接 LoadFile 即完成注入。
// SystemId/CommonSystemProtocol 等由 C++ lua_setglobal 注册、不在 _G.lua 快照中，
// 这里用占位表注入（值为递增整数），保证加载期不做 nil 比较报错；
// 精确协议号待 Ghidra 提取后填入（engine-api.md:136 待提取项）。
func (g *GopherLua) InjectEnums() error {
	dataRoot := g.pre.root
	gPath := filepath.Join(dataRoot, "functions", "_G.lua")

	// 1. _G.lua 1214 常量
	if err := g.LoadFile(gPath); err != nil {
		return fmt.Errorf("加载 _G.lua: %w", err)
	}

	// 2. C++ 注册的全局表占位（SystemId 等）。
	//    先建表并用 DoString 填值，避免逐字段 Register。
	for _, table := range cppGlobalTables {
		if err := g.Load("cppglobals", table.initLua); err != nil {
			return fmt.Errorf("注入 %s: %w", table.name, err)
		}
	}
	return nil
}

// cppGlobalTable 是 C++ 注册、不在 _G.lua 的全局表。
type cppGlobalTable struct {
	name    string
	initLua string
}

// cppGlobalTables 占位定义。值用递增整数（非0、唯一），仅保证加载期可用。
// TODO(ghidra): 从 lua_setglobal 注册点提取精确值后替换。
var cppGlobalTables = []cppGlobalTable{
	{"SystemId", `
if SystemId == nil then
SystemId = {
	enDefaultEntitySystemID = 1,
	enCommonSystemID        = 2,
	enMailSystemID          = 3,
	enMountsSystemID        = 4,
	enEquipSystemID         = 5,
	enRootSystemID          = 6,
	enMsgSystemID           = 7,
	gmSystemID              = 8,
	timeRewardSystem        = 9,
	yunyingActivitySystem   = 10,
	miscsSystem             = 11,
	lianfuSystemId          = 12,
	superExpTimeSystem      = 13,
}
end
`},
	{"CommonSystemProtocol", `
if CommonSystemProtocol == nil then
CommonSystemProtocol = {
	sCountDown      = 1,
	sSendMonEffect  = 2,
	cFightValue     = 3,
}
end
`},
	{"NumericType", `
if NumericType == nil then
NumericType = {
	Chips=1, GuildContrib=2, GuildFund=3, GodWeaponExp=4,
	ShenShouExp=5, Achieve=6,
}
end
`},
}

// EnumValue 读取一个已注入的常量（测试/CI 校验用）。
func (g *GopherLua) EnumValue(name string) (Value, error) {
	return g.DoString(name)
}

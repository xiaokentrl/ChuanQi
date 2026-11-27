package engineapi

import (
	lua "github.com/yuin/gopher-lua"

	"fmt"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// ItemInfo 聚合单个物品的判定字段。
// BagTab 依据 A2 逆向结论（NEXT-STEPS §四）：装备=1/普通=0，判定源=EquipConfig 成员性。
type ItemInfo struct {
	ID      int
	Name    string
	BagTab  int
	UseType int
	Level   int
	ZsLevel int
	Quality int
	Job     int
}

// itemInfoGetFrom 从指定 LState 聚合三表字段（ItemConfig/EquipConfig/ItemDescConfig）。
func itemInfoGetFrom(L *lua.LState, id int) (ItemInfo, bool) {
	var info ItemInfo
	info.ID = id

	cfg, ok := L.GetGlobal("ItemConfig").(*lua.LTable)
	if !ok {
		return info, false
	}
	entry, ok := cfg.RawGetInt(id).(*lua.LTable)
	if !ok {
		return info, false
	}
	info.Name = lua.LVAsString(entry.RawGetString("name"))
	info.UseType = int(lua.LVAsNumber(entry.RawGetString("useType")))
	info.Level = int(lua.LVAsNumber(entry.RawGetString("level")))
	info.ZsLevel = int(lua.LVAsNumber(entry.RawGetString("zsLevel")))
	descIndex := int(lua.LVAsNumber(entry.RawGetString("descIndex")))

	if equip, ok := L.GetGlobal("EquipConfig").(*lua.LTable); ok {
		if equip.RawGetInt(id) != lua.LNil {
			info.BagTab = 1
		}
	}

	if desc, ok := L.GetGlobal("ItemDescConfig").(*lua.LTable); ok && descIndex > 0 {
		if de, ok := desc.RawGetInt(descIndex).(*lua.LTable); ok {
			info.Quality = int(lua.LVAsNumber(de.RawGetString("quality")))
			info.Job = int(lua.LVAsNumber(de.RawGetString("job")))
		}
	}
	return info, true
}

// ItemInfoGetFrom 从指定 VM 聚合物品字段（独立配置 VM 场景）。
func ItemInfoGetFrom(g *luahost.GopherLua, id int) (ItemInfo, bool) {
	return itemInfoGetFrom(g.L, id)
}

// ItemInfoGet 在线引擎入口：从引擎 VM 聚合物品字段。
func (e *Engine) ItemInfoGet(id int) (ItemInfo, bool) {
	if e == nil || e.Lua == nil {
		return ItemInfo{}, false
	}
	return itemInfoGetFrom(e.Lua.L, id)
}

// LoadItemTables 在 VM 中按需加载物品三表（独立引导场景；生产链路由 gameconfig 自动装载）。
func LoadItemTables(g *luahost.GopherLua, paths ...string) error {
	for _, p := range paths {
		if err := g.LoadFile(p); err != nil {
			return fmt.Errorf("加载物品配置 %s 失败: %w", p, err)
		}
	}
	return nil
}

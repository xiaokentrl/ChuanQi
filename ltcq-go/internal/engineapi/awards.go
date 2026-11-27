package engineapi

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// AwardItem 是原版 actorawards.lua giveAwardEx 读取的奖励元素:
// {type, id, count, job}（见 actorawards.lua:22-32）。
type AwardItem struct {
	Type  int
	ID    int
	Count int
	Job   int
}

// decodeAwards 把 Lua 奖励数组表解码为 Go 列表, 语义对齐 actorawards.lua:42-46 的
// ipairs 遍历: 只走数组的 1..n 连续部分, 忽略 hash 字段与 nil 洞之后的元素。
//
// 字段缺失或为 nil 时按 0 读取, 与 giveAwardEx 的 `reward.type or 0` /
// `reward.id or 0` / `reward.job or 0` 默认值一致; count 缺失原版默认为 1,
// 这里只做"原样解码"不注入默认值, 由调用方按原版 giveAwardEx 处理。
//
// 这是 giveAwards 的基础解码能力, 不执行发放, 不接 LActor.giveAwards 注册。
func decodeAwards(v luahost.Value) ([]AwardItem, error) {
	tbl, ok := v.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("奖励表必须是 Lua table, 实际 %T", v)
	}
	// 真正的 ipairs 语义: 从 1 起逐项取, 遇 nil 即停止(不用 Len(), 因 #table 遇 nil 洞不确定)。
	out := make([]AwardItem, 0, tbl.Len())
	for i := 1; ; i++ {
		raw := tbl.RawGetInt(i)
		if raw == lua.LNil {
			break
		}
		row, ok := raw.(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("奖励表第 %d 项不是 Lua table", i)
		}
		out = append(out, AwardItem{
			Type:  int(lua.LVAsNumber(row.RawGetString("type"))),
			ID:    int(lua.LVAsNumber(row.RawGetString("id"))),
			Count: int(lua.LVAsNumber(row.RawGetString("count"))),
			Job:   int(lua.LVAsNumber(row.RawGetString("job"))),
		})
	}
	return out, nil
}

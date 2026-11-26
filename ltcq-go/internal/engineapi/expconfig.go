package engineapi

import (
	"fmt"
	"path/filepath"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	lua "github.com/yuin/gopher-lua"
)

func LoadExpThresholds(dataRoot, ostype string) (map[int]int64, error) {
	g := luahost.NewGopherLua(luahost.Config{DataRoot: dataRoot, OSType: ostype})
	defer g.Close()
	if err := g.LoadFile(filepath.Join(dataRoot, "config", "actor", "exp.config")); err != nil {
		return nil, fmt.Errorf("加载经验配置失败: %w", err)
	}
	table, ok := g.L.GetGlobal("ExpConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("经验配置缺少 ExpConfig 表")
	}
	thresholds := map[int]int64{}
	table.ForEach(func(key, value lua.LValue) {
		entry, ok := value.(*lua.LTable)
		if !ok {
			return
		}
		level := int(lua.LVAsNumber(key))
		exp := int64(lua.LVAsNumber(entry.RawGetString("exp")))
		if level > 0 && exp > 0 {
			thresholds[level] = exp
		}
	})
	if len(thresholds) == 0 {
		return nil, fmt.Errorf("经验配置为空")
	}
	return thresholds, nil
}

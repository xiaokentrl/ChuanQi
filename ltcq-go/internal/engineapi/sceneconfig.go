package engineapi

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
	lua "github.com/yuin/gopher-lua"
)

func LoadSceneWorld(dataRoot, ostype string) (*scene.World, error) {
	g := luahost.NewGopherLua(luahost.Config{DataRoot: dataRoot, OSType: ostype})
	defer g.Close()
	path := filepath.Join(dataRoot, "config", "instance", "scene.config")
	if err := g.LoadFile(path); err != nil {
		return nil, fmt.Errorf("加载场景配置失败: %w", err)
	}
	table, ok := g.L.GetGlobal("ScenesConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("场景配置缺少 ScenesConfig 表")
	}
	configs := map[int]scene.SceneConfig{}
	table.ForEach(func(key, value lua.LValue) {
		entry, ok := value.(*lua.LTable)
		if !ok {
			return
		}
		id := int(lua.LVAsNumber(key))
		cfg := scene.SceneConfig{
			Width:  int(lua.LVAsNumber(entry.RawGetString("sizeX"))) * scene.CellSize,
			Height: int(lua.LVAsNumber(entry.RawGetString("sizeY"))) * scene.CellSize,
			SpawnX: int(lua.LVAsNumber(entry.RawGetString("enterX"))) * scene.CellSize,
			SpawnY: int(lua.LVAsNumber(entry.RawGetString("enterY"))) * scene.CellSize,
		}
		if id <= 0 || cfg.Width <= 0 || cfg.Height <= 0 || cfg.SpawnX < 0 || cfg.SpawnY < 0 || cfg.SpawnX >= cfg.Width || cfg.SpawnY >= cfg.Height {
			slog.Warn("跳过出生点或尺寸非法的场景配置", "scene", id, "config", cfg)
			return
		}
		configs[id] = cfg
	})
	if len(configs) == 0 {
		return nil, fmt.Errorf("场景配置中没有可用场景")
	}
	world := scene.NewWorld()
	if err := world.SetScenes(configs); err != nil {
		return nil, err
	}
	return world, nil
}

package engineapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// 最小三表配置样本：1=武器(装备,.descIndex=7 quality=3)、2=消耗品(普通)
const itemCfgSample = `ItemConfig={
[101001] = { id = 101001, descIndex = 7, name = "testblade", useType = 0, level = 10, zsLevel = 2 },
[101002] = { id = 101002, descIndex = 0, name = "testpotion", useType = 1, level = 1, zsLevel = 0 },
}
EquipConfig={
[101001] = { id = 101001, hp = 75, atk = 22 },
}
ItemDescConfig={
[7] = { id = 7, type = 0, subType = 0, job = 1, quality = 3 },
}`

func writeItemTables(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "config", "item")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "item.config"), []byte(itemCfgSample), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestItemInfoGet(t *testing.T) {
	root := writeItemTables(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := LoadItemTables(g, filepath.Join(root, "config", "item", "item.config")); err != nil {
		t.Fatal(err)
	}

	// 装备：bagTab=1、quality 来自 ItemDescConfig[7]
	blade, ok := ItemInfoGetFrom(g, 101001)
	if !ok {
		t.Fatal("装备物品未命中")
	}
	if blade.BagTab != 1 || blade.Quality != 3 || blade.Job != 1 || blade.Name != "testblade" {
		t.Fatalf("装备字段不符: %+v", blade)
	}
	if blade.Level != 10 || blade.ZsLevel != 2 || blade.UseType != 0 {
		t.Fatalf("装备等级字段不符: %+v", blade)
	}

	// 普通物品：无 EquipConfig 条目 → bagTab=0、无 descIndex → quality=0
	potion, ok := ItemInfoGetFrom(g, 101002)
	if !ok {
		t.Fatal("普通物品未命中")
	}
	if potion.BagTab != 0 || potion.Quality != 0 || potion.Name != "testpotion" {
		t.Fatalf("普通物品字段不符: %+v", potion)
	}

	// 不存在的物品：ok=false 失败关闭
	if _, ok := ItemInfoGetFrom(g, 999999); ok {
		t.Fatal("不存在物品不应命中")
	}
	// 非法输入
	if _, ok := ItemInfoGetFrom(g, 0); ok {
		t.Fatal("非法 id 不应命中")
	}
	if _, ok := ItemInfoGetFrom(g, -1); ok {
		t.Fatal("负数 id 不应命中")
	}
}

func TestItemInfoGetWithoutTables(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{DataRoot: t.TempDir(), OSType: "WIN"})
	defer g.Close()
	if _, ok := ItemInfoGetFrom(g, 101001); ok {
		t.Fatal("无配置表时不应命中")
	}
}

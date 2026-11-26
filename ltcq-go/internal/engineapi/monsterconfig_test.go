package engineapi

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

func writeMonsterCatalog(t *testing.T, root, base, chapter string) {
	t.Helper()
	dir := filepath.Join(root, "config", "monster")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"monsters.config": base, "chaptermonster.config": chapter} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadMonsterCatalogChapterOverridesBase(t *testing.T) {
	root := t.TempDir()
	writeMonsterCatalog(t, root,
		`MonstersConfig={[1]={level=2,hp=100,avatar=10,scale=50},[2]={level=1,hp=300,avatar=20,scale=80}}`,
		`ChapterMonstersConfig={[1]={level=3,hp=200,avatar=11,scale=60}}`,
	)
	catalog, err := LoadMonsterCatalog(root, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 {
		t.Fatalf("怪物数量=%d", len(catalog))
	}
	if got, want := catalog[1], (MonsterConfig{ID: 1, Level: 3, HP: 200}); got != want {
		t.Fatalf("章节覆盖结果=%+v want=%+v", got, want)
	}
	if got, want := catalog[2], (MonsterConfig{ID: 2, Level: 1, HP: 300}); got != want {
		t.Fatalf("基础配置结果=%+v want=%+v", got, want)
	}
}

func TestLoadOriginalMonsterCatalog(t *testing.T) {
	dataRoot := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	if _, err := os.Stat(filepath.Join(dataRoot, "config", "monster", "monsters.config")); os.IsNotExist(err) {
		t.Skip("原版怪物配置不在当前工作区")
	}
	catalog, err := LoadMonsterCatalog(dataRoot, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := catalog[31001], (MonsterConfig{ID: 31001, HP: 127738}); got != want {
		t.Fatalf("原版怪物31001=%+v want=%+v", got, want)
	}
	t.Logf("原版怪物目录加载数量=%d", len(catalog))
}

func TestMonsterManagerSpawnsInPixelCoordinates(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{41: {Width: 83 * scene.CellSize, Height: 63 * scene.CellSize, SpawnX: 77 * scene.CellSize, SpawnY: 22 * scene.CellSize}}); err != nil {
		t.Fatal(err)
	}
	manager := NewMonsterManager(world, map[int]MonsterConfig{
		31001: {ID: 31001, HP: 127738},
	})
	count, err := manager.SpawnAll([]MonsterSpawn{{SceneID: 41, ConfigID: 31001, X: 75, Y: 20, Count: 2}})
	if err != nil || count != 2 {
		t.Fatalf("生成数量=%d err=%v", count, err)
	}
	for i, id := range []int{1 << 30, 1<<30 + 1} {
		entity, ok := world.Entity(id)
		if !ok {
			t.Fatalf("怪物 %d 不存在", id)
		}
		want := scene.Entity{ID: id, Kind: scene.EntityMonster, SceneID: 41, X: 75 * scene.CellSize, Y: 20 * scene.CellSize, ConfigID: 31001, HP: 127738, MaxHP: 127738}
		if entity != want {
			t.Fatalf("第%d只怪物=%+v want=%+v", i, entity, want)
		}
	}
}

func TestMonsterManagerAppliesDevelopmentHPOverride(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	manager := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := manager.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, HP: 500}}); err != nil {
		t.Fatal(err)
	}
	entity, ok := world.Entity(1 << 30)
	if !ok || entity.HP != 500 || entity.MaxHP != 500 {
		t.Fatalf("HP覆盖结果=%+v ok=%v", entity, ok)
	}
}

func TestMonsterManagerValidatesAllSpawnsBeforeCreation(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 64 * scene.CellSize, Height: 64 * scene.CellSize}}); err != nil {
		t.Fatal(err)
	}
	manager := NewMonsterManager(world, map[int]MonsterConfig{
		31001: {ID: 31001, HP: 127738},
		10002: {ID: 10002},
	})
	tests := []MonsterSpawn{
		{SceneID: 99, ConfigID: 31001, X: 1, Y: 1, Count: 1},
		{SceneID: 1, ConfigID: 99999, X: 1, Y: 1, Count: 1},
		{SceneID: 1, ConfigID: 10002, X: 1, Y: 1, Count: 1},
		{SceneID: 1, ConfigID: 31001, X: 64, Y: 1, Count: 1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 0},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, HP: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, Exp: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, Gold: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, RespawnSeconds: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackRange: 128},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, AggroRange: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, ChaseStep: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, LeashRange: -1},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, AggroRange: 256},
		{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, AggroRange: 256, ChaseStep: 64, LeashRange: 128},
	}
	for _, invalid := range tests {
		count, err := manager.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 2, Y: 2, Count: 1}, invalid})
		if err == nil || count != 0 {
			t.Fatalf("非法刷新点应原子拒绝: spawn=%+v count=%d err=%v", invalid, count, err)
		}
		if _, ok := world.Entity(1 << 30); ok {
			t.Fatalf("预校验失败仍生成了怪物: %+v", invalid)
		}
	}
}

func TestLoadMonsterSpawns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spawns.json")
	if err := os.WriteFile(path, []byte(`[{"sceneID":41,"configID":31001,"x":75,"y":20,"count":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	spawns, err := LoadMonsterSpawns(path)
	if err != nil || len(spawns) != 1 || spawns[0] != (MonsterSpawn{SceneID: 41, ConfigID: 31001, X: 75, Y: 20, Count: 1}) {
		t.Fatalf("刷新配置=%+v err=%v", spawns, err)
	}
	if err := os.WriteFile(path, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMonsterSpawns(path); err == nil {
		t.Fatal("空刷新配置应拒绝")
	}
}

func TestBuildMonsterEntityClientLayout(t *testing.T) {
	packet := buildMonsterEntity(scene.Entity{ID: 1 << 30, Kind: scene.EntityMonster, ConfigID: 31001, X: 4800, Y: 1280, HP: 127738, MaxHP: 127738})
	if len(packet) != 63 {
		t.Fatalf("怪物实体长度=%d want=63", len(packet))
	}
	if packet[0] != cmdBase || packet[1] != sBaseCmdOtherEntity || binary.LittleEndian.Uint16(packet[2:4]) != 2 {
		t.Fatalf("怪物实体头错误: %x", packet[:4])
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(packet[4:12])) != float64(1<<30) {
		t.Fatal("怪物 handle 错误")
	}
	if binary.LittleEndian.Uint32(packet[12:16]) != 31001 || math.Float64frombits(binary.LittleEndian.Uint64(packet[16:24])) != 0 {
		t.Fatal("怪物 configID 或 masterHandle 错误")
	}
	if binary.LittleEndian.Uint32(packet[24:28]) != 4800 || binary.LittleEndian.Uint32(packet[28:32]) != 1280 {
		t.Fatal("怪物坐标错误")
	}
	if binary.LittleEndian.Uint16(packet[32:34]) != 3 {
		t.Fatal("怪物属性数量错误")
	}
	for i, want := range []float64{127738, 0, 127738} {
		offset := 34 + i*8
		if got := math.Float64frombits(binary.LittleEndian.Uint64(packet[offset : offset+8])); got != want {
			t.Fatalf("怪物属性%d=%v want=%v", i, got, want)
		}
	}
	if binary.LittleEndian.Uint16(packet[58:60]) != 0 || binary.LittleEndian.Uint16(packet[60:62]) != 0 || packet[62] != 0 {
		t.Fatalf("怪物扩展尾部应为空: %x", packet[58:])
	}
}

func TestMonsterManagerRespawnsWithSameHandleAndFullHP(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	manager := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	spawn := MonsterSpawn{SceneID: 1, ConfigID: 31001, X: 1, Y: 1, Count: 1, HP: 500, RespawnSeconds: 3}
	if _, err := manager.SpawnAll([]MonsterSpawn{spawn}); err != nil {
		t.Fatal(err)
	}
	const id = 1 << 30
	now := time.Unix(1000, 0)
	if !manager.MarkDead(id, now) || manager.MarkDead(id, now) {
		t.Fatal("死亡应只登记一次重生")
	}
	world.RemoveMonster(id)
	if got := manager.RespawnDue(now.Add(2 * time.Second)); len(got) != 0 {
		t.Fatalf("未到重生时间: %v", got)
	}
	if got := manager.RespawnDue(now.Add(3 * time.Second)); len(got) != 1 || got[0] != id {
		t.Fatalf("到期重生列表=%v", got)
	}
	observer := func(delivery scene.Delivery) error {
		packets = append(packets, buildMonsterEntity(delivery.Event.Entity))
		return nil
	}
	if _, err := world.AddPlayer(1, 1, 64, 64, observer); err != nil {
		t.Fatal(err)
	}
	packets = nil
	if err := manager.Respawn(id); err != nil {
		t.Fatal(err)
	}
	entity, ok := world.Entity(id)
	if !ok || entity.HP != 500 || entity.MaxHP != 500 || entity.X != 64 || entity.Y != 64 {
		t.Fatalf("重生实体=%+v ok=%v", entity, ok)
	}
	if len(packets) != 1 || packets[0][0] != cmdBase || packets[0][1] != sBaseCmdOtherEntity {
		t.Fatalf("重生广播错误: %x", packets)
	}
	if got := manager.RespawnDue(now.Add(4 * time.Second)); len(got) != 0 {
		t.Fatalf("重生后不应重复调度: %v", got)
	}
}

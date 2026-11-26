package engineapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

func TestLoadSceneWorld(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "config", "instance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `ScenesConfig={
		[1]={sceneid=1,sizeX=63,sizeY=43,enterX=56,enterY=37},
		[41]={sceneid=41,sizeX=83,sizeY=63,enterX=77,enterY=22},
	}`
	if err := os.WriteFile(filepath.Join(dir, "scene.config"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	world, err := LoadSceneWorld(root, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := world.Spawn(41)
	if err != nil {
		t.Fatal(err)
	}
	want := scene.SceneConfig{Width: 83 * scene.CellSize, Height: 63 * scene.CellSize, SpawnX: 77 * scene.CellSize, SpawnY: 22 * scene.CellSize}
	if cfg != want {
		t.Fatalf("场景41配置=%+v want=%+v", cfg, want)
	}
}

func TestLoadSceneWorldSkipsInvalidEntries(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "config", "instance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `ScenesConfig={
		[1]={sizeX=63,sizeY=43,enterX=56,enterY=37},
		[1005]={sizeX=63,sizeY=51,enterX=63,enterY=23},
	}`
	if err := os.WriteFile(filepath.Join(dir, "scene.config"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	world, err := LoadSceneWorld(root, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.Spawn(1); err != nil {
		t.Fatalf("有效场景未加载: %v", err)
	}
	if _, err := world.Spawn(1005); err == nil {
		t.Fatal("非法场景不应加载")
	}
}

func TestLoadOriginalSceneWorld(t *testing.T) {
	dataRoot := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	if _, err := os.Stat(filepath.Join(dataRoot, "config", "instance", "scene.config")); os.IsNotExist(err) {
		t.Skip("原版场景配置不在当前工作区")
	}
	world, err := LoadSceneWorld(dataRoot, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := world.Spawn(41)
	if err != nil {
		t.Fatal(err)
	}
	want := scene.SceneConfig{Width: 83 * scene.CellSize, Height: 63 * scene.CellSize, SpawnX: 77 * scene.CellSize, SpawnY: 22 * scene.CellSize}
	if cfg != want {
		t.Fatalf("原版场景41=%+v want=%+v", cfg, want)
	}
}

func TestLoadSceneWorldRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "config", "instance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scene.config"), []byte(`ScenesConfig={[1]={sizeX=0,sizeY=43,enterX=56,enterY=37}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSceneWorld(root, "WIN"); err == nil {
		t.Fatal("非法场景配置应拒绝加载")
	}
}

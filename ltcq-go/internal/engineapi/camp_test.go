package engineapi

import (
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestSetGetCamp(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(101)

	for _, camp := range []int{0, 1, 2, 100, 200, 123456, -1} {
		if _, err := g.Call("LActor.setCamp", 101, camp); err != nil {
			t.Fatalf("setCamp(%d) 失败: %v", camp, err)
		}
		got, err := g.Call("LActor.getCamp", 101)
		if err != nil || got.(int) != camp {
			t.Errorf("getCamp=%v err=%v want %d", got, err, camp)
		}
		if actor.Camp != camp {
			t.Errorf("Actor.Camp=%d want %d", actor.Camp, camp)
		}
	}
	if actor.Dirty {
		t.Error("阵营是进程内标签，不应标脏落库")
	}

	if _, err := g.Call("LActor.setCamp", 101, float64(7)); err != nil {
		t.Fatal(err)
	}
	got, err := g.Call("LActor.getCamp", 101)
	if err != nil || got.(int) != 7 {
		t.Errorf("float64 整数值应被接受: got=%v err=%v", got, err)
	}
}

func TestCampFailureClosed(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Camp = 42

	for _, args := range [][]luahost.Value{
		{},
		{101},
		{0, 1},
		{-1, 1},
		{"101", 1},
		{101, "1"},
		{101, 1.5},
		{9999, 1},
	} {
		if _, err := g.Call("LActor.setCamp", args...); err != nil {
			t.Errorf("非法 setCamp 不应抛错: args=%v err=%v", args, err)
		}
	}
	if actor.Camp != 42 {
		t.Fatalf("非法参数修改了阵营: %d want 42", actor.Camp)
	}

	for _, args := range [][]luahost.Value{
		{},
		{0},
		{-1},
		{"101"},
		{9999},
	} {
		got, err := g.Call("LActor.getCamp", args...)
		if err != nil || got.(int) != 0 {
			t.Errorf("非法 getCamp 应返回 0: args=%v got=%v err=%v", args, got, err)
		}
	}
}

package engineapi

import (
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestCheckItemNumNarrow(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Items[1001] = 5

	cases := []struct {
		name string
		args []luahost.Value
		want bool
	}{
		{"enough", []luahost.Value{101, 1001, 3}, true},
		{"exact", []luahost.Value{101, 1001, 5}, true},
		{"short", []luahost.Value{101, 1001, 6}, false},
		{"false flag", []luahost.Value{101, 1001, 3, false}, true},
		{"nil flag", []luahost.Value{101, 1001, 3, nil}, true},
		{"yuanbao flag closed", []luahost.Value{101, 1001, 3, true}, false},
		{"zero count", []luahost.Value{101, 1001, 0}, false},
		{"negative count", []luahost.Value{101, 1001, -1}, false},
		{"bad item", []luahost.Value{101, -1, 1}, false},
		{"offline", []luahost.Value{9999, 1001, 1}, false},
		{"too few args", []luahost.Value{101, 1001}, false},
	}
	for _, tc := range cases {
		got, err := g.Call("LActor.checkItemNum", tc.args...)
		if err != nil {
			t.Fatalf("%s 调用失败: %v", tc.name, err)
		}
		if got.(bool) != tc.want {
			t.Errorf("%s=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestConsumeItemNarrow(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Items[1001] = 5

	got, err := g.Call("LActor.consumeItem", 101, 1001, 2, false, "test")
	if err != nil || got.(bool) != true {
		t.Fatalf("consumeItem=%v err=%v", got, err)
	}
	if actor.Items[1001] != 3 {
		t.Fatalf("扣减后数量=%d want 3", actor.Items[1001])
	}

	blocked := [][]luahost.Value{
		{101, 1001, 5, false, "short"},
		{101, 1001, 1, true, "yuanbao"},
		{101, 1001, 0},
		{101, 1001, -1},
		{101, -1, 1},
		{9999, 1001, 1},
		{101, 1001},
	}
	for _, args := range blocked {
		got, err := g.Call("LActor.consumeItem", args...)
		if err != nil {
			t.Fatalf("非法 consumeItem 不应抛错: args=%v err=%v", args, err)
		}
		if got.(bool) != false {
			t.Errorf("consumeItem%v=%v want false", args, got)
		}
	}
	if actor.Items[1001] != 3 {
		t.Fatalf("失败路径修改了背包: %d want 3", actor.Items[1001])
	}
}

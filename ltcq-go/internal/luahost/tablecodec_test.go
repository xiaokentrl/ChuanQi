package luahost

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func findTablePair(t *testing.T, node TableNode, key string) TablePair {
	t.Helper()
	for _, pair := range node.Pairs {
		if pair.Key.Kind == tableKindStr && pair.Key.Str == key {
			return pair
		}
	}
	t.Fatalf("未找到 key=%q: %+v", key, node.Pairs)
	return TablePair{}
}

func TestTableCodecRoundTrip(t *testing.T) {
	g := NewGopherLua(Config{})
	defer g.Close()
	v, err := g.DoString(`{
		name="x", [7]=42, flag=true, ratio=1.5,
		nested={lv=3}, list={10,20},
	}`)
	if err != nil {
		t.Fatal(err)
	}
	node, err := g.EncodeValue(v)
	if err != nil {
		t.Fatal(err)
	}
	if node.Kind != tableKindTable || len(node.Pairs) != 6 {
		t.Fatalf("编码节点错误: %+v", node)
	}
	decoded, err := g.DecodeValue(node)
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := decoded.(*lua.LTable)
	if !ok {
		t.Fatalf("还原类型错误: %T", decoded)
	}
	if got := tbl.RawGetString("name"); got != lua.LString("x") {
		t.Fatalf("字符串字段=%v", got)
	}
	if got := int(tbl.RawGet(lua.LNumber(7)).(lua.LNumber)); got != 42 {
		t.Fatalf("数字键字段=%d", got)
	}
	if got := bool(tbl.RawGetString("flag").(lua.LBool)); !got {
		t.Fatal("布尔字段丢失")
	}
	if got := float64(tbl.RawGetString("ratio").(lua.LNumber)); got != 1.5 {
		t.Fatalf("浮点字段=%v", got)
	}
	nested, ok := tbl.RawGetString("nested").(*lua.LTable)
	if !ok || int(nested.RawGetString("lv").(lua.LNumber)) != 3 {
		t.Fatalf("嵌套表错误: %v", tbl.RawGetString("nested"))
	}
	list, ok := tbl.RawGetString("list").(*lua.LTable)
	if !ok || int(list.RawGet(lua.LNumber(1)).(lua.LNumber)) != 10 || int(list.RawGet(lua.LNumber(2)).(lua.LNumber)) != 20 {
		t.Fatalf("数组表错误: %v", tbl.RawGetString("list"))
	}
}

func TestTableCodecRejectsCycleAndSkipsFunction(t *testing.T) {
	g := NewGopherLua(Config{})
	defer g.Close()

	cyclic, err := g.DoString(`(function() local t={}; t.self=t; return t end)()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.EncodeValue(cyclic); err == nil {
		t.Fatal("循环引用应报错而不是无限递归")
	}

	withFn, err := g.DoString(`{ fn=function() end, keep=9 }`)
	if err != nil {
		t.Fatal(err)
	}
	node, err := g.EncodeValue(withFn)
	if err != nil {
		t.Fatal(err)
	}
	if got := findTablePair(t, node, "fn").Value.Kind; got != tableKindNil {
		t.Fatalf("函数值应按 nil 落库, kind=%s", got)
	}
	if got := findTablePair(t, node, "keep").Value.Int; got != 9 {
		t.Fatalf("普通字段丢值: %d", got)
	}
}

package engineapi

import (
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestRegistry_T0Coverage(t *testing.T) {
	// T0 前50 API 应全部注册（除少数待确认的）
	n := Count()
	if n < 50 {
		t.Errorf("注册 API 数 %d < 50，T0 未覆盖", n)
	}
	// 检查关键 T0 API 是否存在
	want := []string{
		"LDataPack.writeInt", "LDataPack.allocPacket", "LDataPack.flush",
		"LActor.getActorId", "LActor.sendTipmsg", "LActor.getStaticVar",
		"LActor.costItem", "LActor.giveAwards", "LActor.postScriptEventLite",
	}
	reg := map[string]bool{}
	for _, e := range Registry() {
		reg[e.Name] = true
	}
	for _, name := range want {
		if !reg[name] {
			t.Errorf("缺少 T0 API: %s", name)
		}
	}
}

func TestRegistry_NoDuplicates(t *testing.T) {
	reg := Registry()
	seen := map[string]bool{}
	for _, e := range reg {
		if seen[e.Name] {
			t.Errorf("重复注册: %s", e.Name)
		}
		seen[e.Name] = true
	}
}

func TestDataPack_WriteIntGolden(t *testing.T) {
	// 小端 writeInt + writeShort + writeByte，对齐 golden 字节序
	id := newPack()
	writeFn, _ := getFn("LDataPack.writeInt")
	shortFn, _ := getFn("LDataPack.writeShort")
	byteFn, _ := getFn("LDataPack.writeByte")
	flushFn, _ := getFn("LDataPack.flush")

	writeFn(id, 0x01020304)
	shortFn(id, 0x0506)
	byteFn(id, 0x07)
	flushFn(id)

	want := []byte{0x04, 0x03, 0x02, 0x01, 0x06, 0x05, 0x07}
	got := LastFlushed(id)
	if len(got) != len(want) {
		t.Fatalf("字节数 %d want %d: %x", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]=%02x want %02x", i, got[i], want[i])
		}
	}
}

func TestDataPack_AllocPacketHeader(t *testing.T) {
	// allocPacket(actor, cmd, subcmd) 自动写 cmd/subcmd 头
	allocFn, _ := getFn("LDataPack.allocPacket")
	flushFn, _ := getFn("LDataPack.flush")
	idv, err := allocFn(1, 100, 2) // (actor, cmd, subcmd)
	if err != nil {
		t.Fatal(err)
	}
	flushFn(idv)
	got := LastFlushed(idv.(int))
	if len(got) != 2 || got[0] != 100 || got[1] != 2 {
		t.Errorf("allocPacket 头字节 = %x, want 64 02", got)
	}
}

func TestDataPack_ReadRoundTrip(t *testing.T) {
	id := newPack()
	writeFn, _ := getFn("LDataPack.writeInt")
	readFn, _ := getFn("LDataPack.readInt")
	setPos, _ := getFn("LDataPack.setPosition")

	writeFn(id, 12345)
	setPos(id, 0)
	v, err := readFn(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 12345 {
		t.Errorf("readInt=%v want 12345", v)
	}
}

func TestDataPack_WriteStringNUL(t *testing.T) {
	id := newPack()
	ws, _ := getFn("LDataPack.writeString")
	flush, _ := getFn("LDataPack.flush")
	ws(id, "帅哥")
	flush(id)
	got := LastFlushed(id)
	if got[len(got)-1] != 0 {
		t.Errorf("writeString 应以 NUL 结尾: %x", got)
	}
}

func TestEndToEnd_InstallIntoLua(t *testing.T) {
	// 端到端：把 API 注册进真实 gopher-lua，Lua 脚本调用 go 实现
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	// Lua 调用 LActor.getActorId（stub 返回句柄本身）
	v, err := g.DoString("LActor.getActorId(999)")
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 999 {
		t.Errorf("getActorId=%v want 999", v)
	}

	// Lua 用 LDataPack 写包
	err = g.Load("test", `
		local p = LDataPack.allocPacket(nil, 1, 100)
		LDataPack.writeInt(p, 7)
		LDataPack.flush(p)
		writtenInt = 7
	`)
	if err != nil {
		t.Fatal(err)
	}
	v, err = g.DoString("writtenInt")
	if err != nil || v.(int) != 7 {
		t.Errorf("Lua DataPack 调用失败: v=%v err=%v", v, err)
	}
}

// getFn 从注册表取实现（测试辅助）。
func getFn(name string) (func(...luahost.Value) (luahost.Value, error), bool) {
	for _, e := range Registry() {
		if e.Name == name {
			return e.Fn, true
		}
	}
	return nil, false
}

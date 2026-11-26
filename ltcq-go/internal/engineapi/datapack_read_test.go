package engineapi

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestDataPackReadDataRoundTrip(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	writeHandle := newPack()
	if _, err := g.Call("LDataPack.writeData", writeHandle, 4,
		dtInt, 123456, dtByte, 7, dtString, "abc", dtInt64, int64(5000000000)); err != nil {
		t.Fatal(err)
	}
	pack, err := getPack(writeHandle)
	if err != nil {
		t.Fatal(err)
	}
	readHandle := NewPackFromBytes(append([]byte(nil), pack.buf...))

	script := fmt.Sprintf(`(function() local a,b,c,d = LDataPack.readData(%d,4,%d,%d,%d,%d); return tostring(a).."|"..tostring(b).."|"..tostring(c).."|"..tostring(d) end)()`,
		readHandle, dtInt, dtByte, dtString, dtInt64)
	got, err := g.DoString(script)
	if err != nil {
		t.Fatal(err)
	}
	if want := "123456|7|abc|5000000000"; got != want {
		t.Fatalf("readData=%v want %s", got, want)
	}
}

func TestDataPackReadInt64(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(5000000000))
	handle := NewPackFromBytes(buf)
	got, err := g.Call("LDataPack.readInt64", handle)
	if err != nil {
		t.Fatal(err)
	}
	if got.(int) != 5000000000 {
		t.Fatalf("readInt64=%v want 5000000000", got)
	}
}

func TestDataPackReadFailureClosed(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	cases := [][]luahost.Value{
		{NewPackFromBytes([]byte{1, 2, 3, 4}), 2, dtInt, dtInt},
		{NewPackFromBytes([]byte{1, 2, 3, 4}), 1, 99},
		{NewPackFromBytes([]byte{1, 2, 3, 4}), -1},
	}
	for _, args := range cases {
		if _, err := g.Call("LDataPack.readData", args...); err == nil {
			t.Errorf("readData 非法参数应返回错误: args=%v", args)
		}
	}
	if _, err := g.Call("LDataPack.readInt64", NewPackFromBytes([]byte{1, 2, 3, 4})); err == nil {
		t.Error("readInt64 越界应返回错误")
	}
}

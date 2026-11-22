package netframe

import "fmt"

// 七类日志结构体解码 —— 布局权威来源：DWARF log_def.h（docs/loggerserver-spec.md §四）。
// Body = 结构体裸拷贝（从 actorid_/aid_ 起），字段按偏移切分，定宽 char 数组去尾 \0。
// flag_ & 1 为有效记录标记（对齐原版 Insert* 行为）。

// CounterRec 对应 LCounter（556B，cmd=1）。
type CounterRec struct {
	ActorID    int32
	Aid        string
	UserLevel  string
	Counter    string
	Value      string
	Extra      string
	Kingdom    string
	Phylum     string
	Classfield string
	Family     string
	Genus      string
	SaveDB     bool
	Flag       int32
}

func DecodeCounter(body []byte) (CounterRec, error) {
	var r CounterRec
	if len(body) < 556 {
		return r, errShort("LCounter", len(body), 556)
	}
	r.ActorID = int32(le32(body[0:4]))
	r.Aid = cstr(body[4:68])
	r.UserLevel = cstr(body[68:100])
	r.Counter = cstr(body[100:132])
	r.Value = cstr(body[132:164])
	r.Extra = cstr(body[164:292])
	r.Kingdom = cstr(body[292:324])
	r.Phylum = cstr(body[324:356])
	r.Classfield = cstr(body[356:388])
	r.Family = cstr(body[388:420])
	r.Genus = cstr(body[420:548])
	r.SaveDB = body[548] != 0
	r.Flag = int32(le32(body[552:556]))
	return r, nil
}

// InstallRec 对应 LInstall（292B，cmd=2）。无 actorid_，首字段 aid_（写入 actorid 列，%s）。
type InstallRec struct {
	Aid        string
	Source     string
	Affiliate  string
	Creative   string
	Family     string
	Genus      string
	FromUID    string
	Extra      string
	Flag       int32
}

func DecodeInstall(body []byte) (InstallRec, error) {
	var r InstallRec
	if len(body) < 292 {
		return r, errShort("LInstall", len(body), 292)
	}
	r.Aid = cstr(body[0:64])
	r.Source = cstr(body[64:96])
	r.Affiliate = cstr(body[96:128])
	r.Creative = cstr(body[128:160])
	r.Family = cstr(body[160:192])
	r.Genus = cstr(body[192:224])
	r.FromUID = cstr(body[224:256])
	r.Extra = cstr(body[256:288])
	r.Flag = int32(le32(body[288:292]))
	return r, nil
}

// DauRec 对应 LDau（324B，cmd=3）。
type DauRec struct {
	Aid       string
	Source    string
	Affiliate string
	Creative  string
	Family    string
	Genus     string
	IP        string
	FromUID   string
	Extra     string
	Flag      int32
}

func DecodeDau(body []byte) (DauRec, error) {
	var r DauRec
	if len(body) < 324 {
		return r, errShort("LDau", len(body), 324)
	}
	r.Aid = cstr(body[0:64])
	r.Source = cstr(body[64:96])
	r.Affiliate = cstr(body[96:128])
	r.Creative = cstr(body[128:160])
	r.Family = cstr(body[160:192])
	r.Genus = cstr(body[192:224])
	r.IP = cstr(body[224:256])
	r.FromUID = cstr(body[256:288])
	r.Extra = cstr(body[288:320])
	r.Flag = int32(le32(body[320:324]))
	return r, nil
}

// EconomyRec 对应 LEconomy（1384B，cmd=4）。
type EconomyRec struct {
	ActorID    int32
	Aid        string
	Currency   string
	Amount     string
	Value      string
	Kingdom    string
	Phylum     string
	Classfield string
	Family     string
	Genus      string
	Extra      string
	Extra2     string
	Flag       int32
}

func DecodeEconomy(body []byte) (EconomyRec, error) {
	var r EconomyRec
	if len(body) < 616 { // flag_ 前 616B 即可插入（openkey/pfkey/pfid 不入库）
		return r, errShort("LEconomy", len(body), 616)
	}
	r.ActorID = int32(le32(body[0:4]))
	r.Aid = cstr(body[4:68])
	r.Currency = cstr(body[68:100])
	r.Amount = cstr(body[100:132])
	r.Value = cstr(body[132:164])
	r.Kingdom = cstr(body[164:196])
	r.Phylum = cstr(body[196:228])
	r.Classfield = cstr(body[228:260])
	r.Family = cstr(body[260:292])
	r.Genus = cstr(body[292:324])
	r.Extra = cstr(body[324:452])
	r.Extra2 = cstr(body[452:484])
	r.Flag = int32(le32(body[612:616]))
	return r, nil
}

// MilestoneRec 对应 LMilestone（168B，cmd=5）。
type MilestoneRec struct {
	ActorID   int32
	Aid       string
	Milestone string
	Value     string
	Extra     string
}

func DecodeMilestone(body []byte) (MilestoneRec, error) {
	var r MilestoneRec
	if len(body) < 164 {
		return r, errShort("LMilestone", len(body), 164)
	}
	r.ActorID = int32(le32(body[0:4]))
	r.Aid = cstr(body[4:68])
	r.Milestone = cstr(body[68:100])
	r.Value = cstr(body[100:132])
	r.Extra = cstr(body[132:164])
	return r, nil
}

// ChatRec 对应 LChat（1288B，cmd=14）。
type ChatRec struct {
	ActorID int32
	Aid     string
	Name    string
	IP      string
	PF      string
	Type    int32
	ToAid   string
	ToName  string
	Content string
}

func DecodeChat(body []byte) (ChatRec, error) {
	var r ChatRec
	if len(body) < 1288 {
		return r, errShort("LChat", len(body), 1288)
	}
	r.ActorID = int32(le32(body[0:4]))
	r.Aid = cstr(body[4:68])
	r.Name = cstr(body[68:100])
	r.IP = cstr(body[100:132])
	r.PF = cstr(body[132:164])
	r.Type = int32(le32(body[164:168]))
	r.ToAid = cstr(body[168:232])
	r.ToName = cstr(body[232:264])
	r.Content = cstr(body[264:1288])
	return r, nil
}

// OnlineRec 对应 cmd=9 的 payload（2B cmd + 2×int32；Lua precessLog 等价）。
type OnlineRec struct {
	Count    int32
	TmpCount int32
}

func DecodeOnline(body []byte) (OnlineRec, error) {
	var r OnlineRec
	if len(body) < 8 {
		return r, errShort("Online", len(body), 8)
	}
	r.Count = int32(le32(body[0:4]))
	r.TmpCount = int32(le32(body[4:8]))
	return r, nil
}

func le32(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24 }

func errShort(name string, got, want int) error {
	return fmt.Errorf("netframe: %s body 过短 got=%d want=%d", name, got, want)
}

package engineapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// A4 持久化槽位：actorvariable.cs_variable（跨服静态变量数据）。
//
// 选择原因：actorvariable.variable 已被背包以 "LTGOBAG1" magic-tail 使用，
// storedata/together_hit_equip 语义明确属于商店/合击装备；cs_variable 同样
// 是 staticVar 类 blob，且 Go 隔离库由本服务独占，避免覆盖现网 staticVar 前缀。
// 格式是 Go 自有 JSON 存档，不兼容原版 C++ 私有 blob（ADR-006 已决定不做旧档兼容）。
const (
	actorPersistVersion = 1
	actorPersistMagic   = "LTGOPERS1"
)

type actorPersistData struct {
	Version      int                `json:"v"`
	ClientConfig map[int]int        `json:"clientConfig,omitempty"`
	Skills       map[int]int        `json:"skills,omitempty"`
	Static       *luahost.TableNode `json:"static,omitempty"`
}

type actorPersistStore interface {
	LoadActorPersist(actorID int) (*actorPersistData, error)
	SaveActorPersist(actorID int, data *actorPersistData) error
}

func defaultActorPersistData() *actorPersistData {
	return &actorPersistData{Version: actorPersistVersion, ClientConfig: map[int]int{}, Skills: map[int]int{}}
}

func (s *MySQLAccountStore) LoadActorPersist(actorID int) (*actorPersistData, error) {
	var blob []byte
	err := s.db.QueryRow("SELECT cs_variable FROM actorvariable WHERE actorid=?", actorID).Scan(&blob)
	if err == sql.ErrNoRows {
		return defaultActorPersistData(), nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(blob, []byte(actorPersistMagic)) {
		return defaultActorPersistData(), nil
	}
	var data actorPersistData
	if err := json.Unmarshal(blob[len(actorPersistMagic):], &data); err != nil {
		return nil, fmt.Errorf("角色持久化数据解码失败: %w", err)
	}
	if data.ClientConfig == nil {
		data.ClientConfig = map[int]int{}
	}
	if data.Skills == nil {
		data.Skills = map[int]int{}
	}
	return &data, nil
}

func encodeActorPersist(data *actorPersistData) ([]byte, error) {
	if data == nil {
		data = defaultActorPersistData()
	}
	data.Version = actorPersistVersion
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return append([]byte(actorPersistMagic), payload...), nil
}

func (s *MySQLAccountStore) SaveActorPersist(actorID int, data *actorPersistData) error {
	blob, err := encodeActorPersist(data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO actorvariable(actorid, cs_variable) VALUES(?,?)
		ON DUPLICATE KEY UPDATE cs_variable=VALUES(cs_variable)`, actorID, blob)
	return err
}

func (s *MySQLAccountStore) SaveSkillUpgrade(actorID int, gold int64, data *actorPersistData) error {
	if actorID <= 0 || gold < 0 {
		return fmt.Errorf("技能升级存档参数非法: actor=%d gold=%d", actorID, gold)
	}
	blob, err := encodeActorPersist(data)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE actors SET gold=?, updatetime=NOW() WHERE actorid=?", gold, actorID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		var exists int
		if err := tx.QueryRow("SELECT 1 FROM actors WHERE actorid=?", actorID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return fmt.Errorf("技能升级未命中角色: actor=%d", actorID)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO actorvariable(actorid, cs_variable) VALUES(?,?)
		ON DUPLICATE KEY UPDATE cs_variable=VALUES(cs_variable)`, actorID, blob); err != nil {
		return err
	}
	return tx.Commit()
}

func tableCodecOf(L luahost.Interpreter) luahost.TableCodec {
	codec, _ := L.(luahost.TableCodec)
	return codec
}

func markStaticDirty(actor *Actor) {
	if actor == nil {
		return
	}
	actor.mu.Lock()
	actor.Dirty = true
	actor.StaticDirty = true
	actor.mu.Unlock()
}

// applyActorPersist 用落库数据覆盖内存字段。直接赋值而不走 setter，
// 避免把加载动作误判为新的未落库修改。
func (a *Actor) applyActorPersist(data *actorPersistData, static luahost.Value) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg := map[int]int{}
	skills := map[int]int{}
	if data != nil {
		for key, val := range data.ClientConfig {
			cfg[key] = val
		}
		for skillID, level := range data.Skills {
			skills[skillID] = level
		}
	}
	a.ClientConfig = cfg
	a.Skills = skills
	a.Static = static
	a.ClientConfigDirty = false
	a.SkillsDirty = false
	a.StaticDirty = false
	a.Dirty = false
}

func restoreActorPersist(actor *Actor, store AccountStore, codec luahost.TableCodec) error {
	if actor == nil {
		return nil
	}
	persistStore, ok := store.(actorPersistStore)
	if !ok {
		return nil
	}
	data, err := persistStore.LoadActorPersist(actor.ID)
	if err != nil {
		return fmt.Errorf("角色持久化数据加载失败: actor=%d %w", actor.ID, err)
	}
	var static luahost.Value
	if data != nil && data.Static != nil {
		if codec == nil {
			return fmt.Errorf("角色 %d 存在 Static 存档但未安装 Lua 表解码器", actor.ID)
		}
		decoded, err := codec.DecodeValue(*data.Static)
		if err != nil {
			return fmt.Errorf("角色 %d Static 存档还原失败: %w", actor.ID, err)
		}
		static = decoded
	}
	actor.applyActorPersist(data, static)
	return nil
}

// flushDirty 把 ClientConfig、Skills 与 Static 当前快照一次落库。
// Static 每次都随快照写入，避免只更新其他字段时抹掉已还原的 staticVar。
func flushDirty(actor *Actor, store AccountStore, codec luahost.TableCodec) error {
	if actor == nil {
		return nil
	}
	persistStore, ok := store.(actorPersistStore)
	if !ok {
		return nil
	}
	actor.mu.Lock()
	defer actor.mu.Unlock()
	if !actor.ClientConfigDirty && !actor.SkillsDirty && !actor.StaticDirty {
		return nil
	}
	data := actorPersistData{
		Version:      actorPersistVersion,
		ClientConfig: map[int]int{},
		Skills:       map[int]int{},
	}
	for key, val := range actor.ClientConfig {
		data.ClientConfig[key] = val
	}
	for skillID, level := range actor.Skills {
		data.Skills[skillID] = level
	}
	if actor.Static != nil {
		if codec == nil {
			return fmt.Errorf("角色 %d Static 未落库：缺少 Lua 表编码器", actor.ID)
		}
		node, err := codec.EncodeValue(actor.Static)
		if err != nil {
			return fmt.Errorf("角色 %d Static 编码失败: %w", actor.ID, err)
		}
		data.Static = &node
	}
	if err := persistStore.SaveActorPersist(actor.ID, &data); err != nil {
		return fmt.Errorf("角色持久化数据保存失败: actor=%d %w", actor.ID, err)
	}
	actor.ClientConfigDirty = false
	actor.SkillsDirty = false
	actor.StaticDirty = false
	actor.Dirty = false
	return nil
}

// newClientConfigHandler 在原 ClientConfig 更新逻辑后追加同步落库，
// 保证 cBaseCmd_ClientConfig(19) 请求处理后的配置在重登时仍存在。
func newClientConfigHandler(store AccountStore, codec luahost.TableCodec) func(sid uint16, actorID int, body []byte) error {
	return func(sid uint16, actorID int, body []byte) error {
		if err := handleClientConfig(sid, actorID, body); err != nil {
			return err
		}
		return flushDirty(DefaultActorManager.Get(actorID), store, codec)
	}
}

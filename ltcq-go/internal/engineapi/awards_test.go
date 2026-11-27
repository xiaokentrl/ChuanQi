package engineapi

import (
	"reflect"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestDecodeAwards(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()

	cases := []struct {
		name    string
		lua     string
		want    []AwardItem
		wantErr bool
	}{
		{
			name: "type_id_count_job",
			lua:  `{{type=1,id=200156,count=2,job=3},{type=0,id=1,count=500000}}`,
			want: []AwardItem{
				{Type: 1, ID: 200156, Count: 2, Job: 3},
				{Type: 0, ID: 1, Count: 500000, Job: 0},
			},
		},
		{
			name: "missing_fields_default_zero",
			lua:  `{{id=101001}, {type=1,count=3}}`,
			want: []AwardItem{
				{Type: 0, ID: 101001, Count: 0, Job: 0},
				{Type: 1, ID: 0, Count: 3, Job: 0},
			},
		},
		{
			name: "extra_hash_fields_ignored",
			lua:  `{{type=1,id=5,count=1,job=0,extra=99}, foo=123}`,
			want: []AwardItem{{Type: 1, ID: 5, Count: 1, Job: 0}},
		},
		{
			name: "empty_table",
			lua:  `{}`,
			want: []AwardItem{},
		},
		{
			name:    "nil_hole_stops_ipairs",
			lua:     `{{type=0,id=1,count=1}, nil, {type=1,id=2,count=1}}`,
			wantErr: false,
			want:    []AwardItem{{Type: 0, ID: 1, Count: 1}},
		},
		{
			name:    "element_not_table",
			lua:     `{123}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		v, err := g.DoString(tc.lua)
		if err != nil {
			t.Fatalf("%s Lua 构造失败: %v", tc.name, err)
		}
		got, err := decodeAwards(v)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s decodeAwards err=%v wantErr=%v", tc.name, err, tc.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s got=%+v want=%+v", tc.name, got, tc.want)
		}
	}
}

func TestDecodeAwardsRejectsNonTable(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()

	for _, src := range []string{`42`, `"x"`, `nil`} {
		v, err := g.DoString(src)
		if err != nil {
			t.Fatalf("Lua 构造失败 %s: %v", src, err)
		}
		if _, err := decodeAwards(v); err == nil {
			t.Errorf("decodeAwards(%s) 应拒绝非 table", src)
		}
	}
}

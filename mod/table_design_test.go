package mod

import (
	"reflect"
	"testing"
)

func TestGetDescAndSuggestByLevel(t *testing.T) {
	type args struct {
		level    int
		partType string
		alerType string
		location string
	}
	tests := []struct {
		name        string
		args        args
		wantDesc    string
		wantSuggest string
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{level: 0, partType: "主轴承", alerType: "F", location: "1"}, wantDesc: "振动幅值趋势平稳；无明显轴承故障频率", wantSuggest: "建议正常运行"},
		{name: "test2", args: args{level: 1, partType: "主轴承", alerType: "F", location: "1"}, wantDesc: "振动幅值趋势平稳；无明显轴承故障频率", wantSuggest: "建议正常运行"},
		{name: "test3", args: args{level: 2, partType: "主轴承", alerType: "F", location: "1"}, wantDesc: "1频域残差模型振幅超限", wantSuggest: "建议注脂改善润滑"},
		{name: "test4", args: args{level: 3, partType: "主轴承", alerType: "F", location: "1"}, wantDesc: "1频域残差模型振幅报警", wantSuggest: "建议检查主轴振动和异响情况"},
		{name: "test3", args: args{level: 2, partType: "主轴承", alerType: "T", location: "1"}, wantDesc: "1时域残差模型振幅超限", wantSuggest: "建议注脂改善润滑"},
		{name: "test4", args: args{level: 3, partType: "主轴承", alerType: "T", location: "1"}, wantDesc: "1时域残差模型振幅报警", wantSuggest: "建议检查主轴振动和异响情况"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDesc, gotSuggest := GetDescAndSuggestByLevel(tt.args.level, tt.args.partType, tt.args.alerType, tt.args.location)
			if gotDesc != tt.wantDesc {
				t.Errorf("GetDescAndSuggestByLevel() gotDesc = %v, want %v", gotDesc, tt.wantDesc)
			}
			if gotSuggest != tt.wantSuggest {
				t.Errorf("GetDescAndSuggestByLevel() gotSuggest = %v, want %v", gotSuggest, tt.wantSuggest)
			}
		})
	}
}

func TestUser_WindfarmIdsArrToStr(t *testing.T) {
	type fields struct {
		ID             uint
		Username       string
		Password       string
		Level          uint8
		WindfarmIdsStr string
		WindfarmIds    []int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
		{name: "test1", fields: fields{WindfarmIds: []int{}}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{
				ID:             tt.fields.ID,
				Username:       tt.fields.Username,
				Password:       tt.fields.Password,
				Level:          tt.fields.Level,
				WindfarmIdsStr: tt.fields.WindfarmIdsStr,
				WindfarmIds:    tt.fields.WindfarmIds,
			}
			u.WindfarmIdsArrToStr()
			if u.WindfarmIdsStr != tt.want {
				t.Errorf("User.WindfarmIdsArrToStr() = %v, want %v", u.WindfarmIdsStr, tt.want)
			}
		})
	}
}

func TestUser_WinfarmIdsStrToArr(t *testing.T) {
	type fields struct {
		ID             uint
		Username       string
		Password       string
		Level          uint8
		WindfarmIdsStr string
		WindfarmIds    []int
	}
	tests := []struct {
		name   string
		fields fields
		want   []int
	}{
		// TODO: Add test cases.
		{name: "test1", fields: fields{WindfarmIdsStr: "1,2,3,4"}, want: []int{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{
				ID:             tt.fields.ID,
				Username:       tt.fields.Username,
				Password:       tt.fields.Password,
				Level:          tt.fields.Level,
				WindfarmIdsStr: tt.fields.WindfarmIdsStr,
				WindfarmIds:    tt.fields.WindfarmIds,
			}
			u.WindfarmIdsStrToArr()
			if !reflect.DeepEqual(u.WindfarmIds, tt.want) {
				t.Errorf("User.WindfarmIdsStrToArr() = %v, want %v", u.WindfarmIds, tt.want)
			}
		})
	}
}

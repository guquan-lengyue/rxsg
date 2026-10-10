//go:build integration

package hero

import (
	"context"
	"strings"
	"testing"
)

// office_info_integration_test.go 真库集成测试：R11-3 官署面板聚合读取（OfficeFunc.php）。
func TestOfficeInfo(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 无官署建筑（bid=11）→ getOfficeInfo.no_office_built（lang 原文 "铁锭"，错位保留）
	if _, err := s.OfficeInfo(ctx, f.UID, f.CID); err == nil || !strings.Contains(err.Error(), msgNoOfficeBuilt) {
		t.Fatalf("no office want %q, got %v", msgNoOfficeBuilt, err)
	}

	// 建 5 级官署；城内 2 名将领
	if _, err := f.DB.Exec(ctx, `insert into buildings
		(city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,11,'c1',5,0,0,0)`, f.CID); err != nil {
		t.Fatalf("insert office: %v", err)
	}
	out, err := s.OfficeInfo(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("OfficeInfo: %v", err)
	}
	if out.OfficeLevel != 5 {
		t.Fatalf("office_level want 5, got %d", out.OfficeLevel)
	}
	if out.ValidPosition != 3 { // 5 - 2 将
		t.Fatalf("valid_position want 3, got %d", out.ValidPosition)
	}
	if out.ChiefHeroID != f.HID1 || out.GeneralHeroID != f.HID1 {
		t.Fatalf("chief/general want %d, got %d/%d", f.HID1, out.ChiefHeroID, out.GeneralHeroID)
	}
	if out.CounsellorHeroID != 0 {
		t.Fatalf("counsellor want 0, got %d", out.CounsellorHeroID)
	}
	if len(out.Heroes) != 2 {
		t.Fatalf("heroes want 2, got %d", len(out.Heroes))
	}
	if out.Nobility != 0 {
		t.Fatalf("nobility want 0, got %d", out.Nobility)
	}

	// 推恩令（buftype=16, bufparam=5）提升爵位：3 → 8
	if _, err := f.DB.Exec(ctx, "update users set nobility='3' where id=?", f.UID); err != nil {
		t.Fatalf("set nobility: %v", err)
	}
	if _, err := f.DB.Exec(ctx, "insert into user_buffers (user_id,buftype,bufparam,endtime) values (?,16,5,0)", f.UID); err != nil {
		t.Fatalf("insert buffer: %v", err)
	}
	out2, err := s.OfficeInfo(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("OfficeInfo(2): %v", err)
	}
	if out2.Nobility != 8 {
		t.Fatalf("nobility with tuien want 8, got %d", out2.Nobility)
	}
}

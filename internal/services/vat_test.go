package services

import (
	"testing"
)

func TestGetVATRateByMarketplace(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	// 1. 测试 US (应该没有对应的 EU VAT)
	rate, cc := GetVATRateByMarketplace(db, 1, "ATVPDKIKX0DER") // 北美站 ID
	if rate != 0 || cc != "" {
		t.Errorf("US should have no VAT defaults, got rate %v, country %v", rate, cc)
	}

	// 2. 测试默认 DE 德国站预期标准税率
	rate, cc = GetVATRateByMarketplace(db, 1, "A1PA6795UKMFR9") // 德国站 ID
	if rate != 19.0 || cc != "DE" {
		t.Errorf("DE defaults to 19.0, got %v (%s)", rate, cc)
	}

	// 3. 测试插入用户自定义覆盖税度
	err := SaveCustomVATRate(db, 1, "DE", 15.0)
	if err != nil {
		t.Fatalf("Save custom VAT error: %v", err)
	}

	rate, cc = GetVATRateByMarketplace(db, 1, "A1PA6795UKMFR9")
	if rate != 15.0 {
		t.Errorf("Expected custom rate 15.0, got %v", rate)
	}

	// 4. 测试恢复系统默认重置功能
	err = ResetVATRate(db, 1, "DE")
	if err != nil {
		t.Fatalf("Reset custom VAT error: %v", err)
	}

	rate, cc = GetVATRateByMarketplace(db, 1, "A1PA6795UKMFR9")
	if rate != 19.0 {
		t.Errorf("Expected reset reverting rule to 19.0, got %v", rate)
	}

	// 5. 非正常拦截（极值测试拦截）
	err = SaveCustomVATRate(db, 1, "DE", 60.0)
	if err == nil {
		t.Errorf("Expected threshold guard validation error for rate > 50, but it allowed.")
	}
}

func TestCalcVATBreakdown(t *testing.T) {
	// 一件售价 120 欧元的商品，其中附带 20% 的 VAT。净价应当是 100 欧，缴税区是 20 欧。
	b := CalcVATFromInclusive(120.0, 20.0)
	
	if b.ExclusivePrice != 100.0 {
		t.Errorf("Expected exclusive 100, got %v", b.ExclusivePrice)
	}
	if b.VATAmount != 20.0 {
		t.Errorf("Expected vat size 20, got %v", b.VATAmount)
	}
}

package market

import ("strings";"testing")

func TestMarketOutputLanguage(t *testing.T){
 if !strings.Contains(instruction,"简体中文"){t.Fatal("market report evidence and risk must be Chinese")}
 if !strings.Contains(instruction,"RISK_OFF"){t.Fatal("typed regime enum must be unchanged")}
}

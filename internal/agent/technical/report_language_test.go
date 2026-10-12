package technical

import ("strings";"testing")

func TestTechnicalOutputLanguage(t *testing.T){
 if !strings.Contains(instruction,"简体中文"){t.Fatal("technical evidence and risk must be Chinese")}
 if !strings.Contains(instruction,"NO_DATA"){t.Fatal("typed status enum must be unchanged")}
}

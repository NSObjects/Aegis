// Package researchlang enforces the language of newly authored research
// narrative fields. Source quotes, identifiers, schema keys and old artifacts
// must never be passed to this detector or rewritten as a "fix".
package researchlang

import (
 "context"
 "errors"
 "fmt"
 "unicode"
)

type strictKey struct{}

// RequireChinese marks a NEW live research invocation. Existing archives and
// standalone validators are not retroactively altered.
func RequireChinese(ctx context.Context) context.Context {
 return context.WithValue(ctx,strictKey{},true)
}

func Required(ctx context.Context) bool {
 if ctx==nil{return false}
 enabled,_:=ctx.Value(strictKey{}).(bool)
 return enabled
}

type Field struct{ Name, Text string }

var ErrLanguage = errors.New("research narrative is not in simplified Chinese")

const Correction = "上一轮研究叙述包含明显的英文正文。请基于完全相同的已验证输入重新生成完整 JSON：summary、thesis、claim、description、risk 等分析性文字全部使用简体中文。保留原有事实、引用、ID、日期、数值、JSON 字段名和枚举，不要擅自改动外部证据引文，不要编造或省略内容。"

// Check detects clear English prose, not individual English terms or codes.
// It is deliberately conservative: an ASCII phrase of at least four normal
// words AND 22 letters is required. This heuristic is not a language oracle.
func Check(ctx context.Context, fields ...Field) error {
 if !Required(ctx){return nil}
 for _,field:=range fields{
  if englishProse(field.Text) {
   return fmt.Errorf("%w: %s",ErrLanguage,field.Name)
  }
 }
 return nil
}

func englishProse(s string) bool {
 words, letters:=0,0
 inWord:=false
 token:=make([]rune,0,32)
 finishWord:=func(){
  if !inWord{return}
  // Uppercase protocol tokens (MACD, RISK_OFF etc.) are not prose.
  allCaps:=true
  for _,r:=range token {if unicode.IsLower(r){allCaps=false;break}}
  if !allCaps && len(token)>=2 {words++;letters+=len(token)}
  inWord=false
  token=token[:0]
 }
 sentenceEnd:=func() bool {
  finishWord()
  bad:=words>=4 && letters>=22
  words,letters=0,0
  return bad
 }
 for _,r:=range s {
  if r<128 && unicode.IsLetter(r) {
   inWord=true
   token=append(token,r)
   continue
  }
  finishWord()
  if unicode.Is(unicode.Han,r)||r=='。'||r=='！'||r=='？'||r=='.'||r=='!'||r=='?'||r=='；'||r==';'||r==','||r=='，'||r=='\n'||r==':'||r=='：' {
   if sentenceEnd(){return true}
  }
  if unicode.IsDigit(r)||r=='_'||r=='/' {words,letters=0,0}
 }
 return sentenceEnd()
}

package technical

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "strings"
 "time"
)

const defaultTushareURL = "https://api.tushare.pro"

type TushareClient struct { token, endpoint string; client *http.Client }
type tushareRequest struct { APIName string `json:"api_name"`; Token string `json:"token"`; Params map[string]any `json:"params"`; Fields string `json:"fields"` }
type tushareResponse struct { Code int `json:"code"`; Msg string `json:"msg"`; Data struct { Fields []string `json:"fields"`; Items [][]any `json:"items"` } `json:"data"` }

func NewTushareClient(token string) (*TushareClient,error) {
 token=strings.TrimSpace(token); if token=="" { return nil,errors.New("tushare token is required") }
 return &TushareClient{token:token,endpoint:defaultTushareURL,client:&http.Client{Timeout:20*time.Second}},nil
}
func (c *TushareClient) query(ctx context.Context, api string, params map[string]any, fields ...string)([]map[string]any,error){
 body,err:=json.Marshal(tushareRequest{APIName:api,Token:c.token,Params:params,Fields:strings.Join(fields,",")});if err!=nil{return nil,err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,c.endpoint,bytes.NewReader(body));if err!=nil{return nil,err};req.Header.Set("Content-Type","application/json")
 resp,err:=c.client.Do(req);if err!=nil{return nil,fmt.Errorf("tushare %s request: %w",api,err)};defer resp.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,8<<20));if err!=nil{return nil,err};if resp.StatusCode<200||resp.StatusCode>=300{return nil,fmt.Errorf("tushare %s http status %d: %s",api,resp.StatusCode,strings.TrimSpace(string(raw)))}
 var out tushareResponse;if err:=json.Unmarshal(raw,&out);err!=nil{return nil,fmt.Errorf("decode tushare %s response: %w",api,err)};if out.Code!=0{return nil,fmt.Errorf("tushare %s error code=%d: %s",api,out.Code,out.Msg)}
 rows:=make([]map[string]any,0,len(out.Data.Items));for _,item:=range out.Data.Items { row:=make(map[string]any,len(out.Data.Fields));for i,k:=range out.Data.Fields {if i<len(item){row[k]=item[i]}};rows=append(rows,row)}
 return rows,nil
}

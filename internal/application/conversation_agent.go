package application

import (
 "context"
 "errors"

 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/components/tool"
 "github.com/cloudwego/eino/compose"
)

const conversationInstruction = `你是 Aegis 金融研究工作台的对话助手。只能依据工具返回的已持久化研究记录回答个股研究事实，引用具体 run_id。
用户要求查看研究状态时使用 GetResearchRun；要求检索历史使用 ListResearchRuns；要求结论依据时使用 ReadArtifact；要求多空观点时使用 ReadDebate。
不要将旧 Run 的研究时点说成当前行情，不得自行编造价格、证据、账户或交易权限。工具失败时说明数据不可用。
StartResearch 会触发新的外部研究并可能产生模型费用；只能在用户明确要求新研究且服务端授权后使用。不得进行真实下单。回答使用简体中文。`

// NewConversationAgent wires Eino's own ReAct tool loop to the same persisted
// ResearchRun service as the API. A request-scoped policy controls whether a
// new potentially paid research run is available at all.
func NewConversationAgent(ctx context.Context, chatModel model.ToolCallingChatModel, researchTools *ConversationResearchTools, startPolicy *ConversationStartPolicy) (adk.Agent,error) {
 if ctx==nil {return nil,errors.New("conversation context is required")}
 if chatModel==nil {return nil,errors.New("conversation chat model is required")}
 if researchTools==nil {return nil,errors.New("conversation research tools are required")}
 reads,err:=researchTools.EinoReadTools()
 if err!=nil{return nil,err}
 baseTools:=make([]tool.BaseTool,0,len(reads)+1)
 for _,read:=range reads {baseTools=append(baseTools,read)}
 if startPolicy!=nil {
  start,err:=researchTools.EinoStartResearchTool(*startPolicy)
  if err!=nil{return nil,err}
  baseTools=append(baseTools,start)
 }
 return adk.NewChatModelAgent(ctx,&adk.ChatModelAgentConfig{
  Name:"AegisConversation",
  Description:"Grounded research conversation using persisted Aegis ResearchRuns",
  Instruction:conversationInstruction,
  Model:chatModel,
  ToolsConfig:adk.ToolsConfig{ToolsNodeConfig:compose.ToolsNodeConfig{Tools:baseTools}},
  MaxIterations:8,
 })
}

package application

import (
 "crypto/rand"
 "encoding/hex"
 "errors"
 "regexp"
)

var conversationSessionIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// NewConversationSessionID returns a cryptographically random opaque identifier.
// Session IDs are routing keys, not credentials or proof of ownership.
func NewConversationSessionID()(string,error){
 var id [16]byte
 if _,err:=rand.Read(id[:]);err!=nil{return "",err}
 return hex.EncodeToString(id[:]),nil
}

func ValidateConversationSessionID(id string)error{
 if !conversationSessionIDPattern.MatchString(id){return errors.New("invalid conversation session identifier")}
 return nil
}

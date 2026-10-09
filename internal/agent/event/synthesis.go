package event

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type AnalystSynthesis struct {
	Summary                string             `json:"summary"`
	OverallMateriality     MaterialityLevel   `json:"overall_materiality"`
	OverallDirection       ImpactDirection    `json:"overall_direction"`
	DominantImpactChannels []ImpactChannel    `json:"dominant_impact_channels,omitempty"`
	KeyEventIDs            []string           `json:"key_event_ids"`
	KeyRisks               []string           `json:"key_risks,omitempty"`
	Confidence             float64            `json:"confidence"`
}

func normalizeAnalystSynthesis(raw AnalystSynthesis, events []Event) (AnalystSynthesis,error) {
	raw.Summary=strings.TrimSpace(raw.Summary)
	if raw.Summary=="" { return AnalystSynthesis{},errors.New("analyst synthesis summary is required") }
	if !validMaterialityLevel(raw.OverallMateriality) { return AnalystSynthesis{},fmt.Errorf("analyst synthesis has invalid materiality %q",raw.OverallMateriality) }
	if !validImpactDirection(raw.OverallDirection) { return AnalystSynthesis{},fmt.Errorf("analyst synthesis has invalid direction %q",raw.OverallDirection) }
	if math.IsNaN(raw.Confidence) || raw.Confidence<0 || raw.Confidence>1 { return AnalystSynthesis{},fmt.Errorf("analyst synthesis confidence out of range: %f",raw.Confidence) }

	eventIDs:=make(map[string]struct{},len(events))
	for _,event:=range events { eventIDs[event.ID]=struct{}{} }
	raw.KeyEventIDs=uniqueSortedStrings(raw.KeyEventIDs)
	if len(raw.KeyEventIDs)==0 { return AnalystSynthesis{},errors.New("analyst synthesis requires key event ids") }
	for _,id:=range raw.KeyEventIDs {
		if _,ok:=eventIDs[id];!ok { return AnalystSynthesis{},fmt.Errorf("analyst synthesis references unknown event %q",id) }
	}
	channels:=make([]ImpactChannel,0,len(raw.DominantImpactChannels))
	seenChannels:=map[ImpactChannel]struct{}{}
	for _,channel:=range raw.DominantImpactChannels {
		if !validImpactChannel(channel) { return AnalystSynthesis{},fmt.Errorf("analyst synthesis has invalid impact channel %q",channel) }
		if _,ok:=seenChannels[channel];ok { continue }
		seenChannels[channel]=struct{}{}
		channels=append(channels,channel)
	}
	sort.Slice(channels,func(i,j int)bool{return channels[i]<channels[j]})
	raw.DominantImpactChannels=channels
	allowedChannels:=map[ImpactChannel]struct{}{}
	for _,event:=range events { for _,mechanism:=range event.ImpactMechanisms { allowedChannels[mechanism.Channel]=struct{}{} } }
	for _,channel:=range channels { if _,ok:=allowedChannels[channel];!ok { return AnalystSynthesis{},fmt.Errorf("analyst synthesis impact channel %q is not present in event mechanisms",channel) } }
	raw.KeyRisks=uniqueSortedStrings(raw.KeyRisks)
	return raw,nil
}

func validateAnalystSynthesis(v AnalystSynthesis, events []Event) error {
	normalized,err:=normalizeAnalystSynthesis(v,events)
	if err!=nil{return err}
	if !equalStringSlices(impactChannelsToStrings(v.DominantImpactChannels),impactChannelsToStrings(normalized.DominantImpactChannels)) {
		return errors.New("analyst synthesis dominant impact channels must be unique and sorted")
	}
	if !equalStringSlices(v.KeyEventIDs,normalized.KeyEventIDs) { return errors.New("analyst synthesis key event ids must be unique and sorted") }
	if !equalStringSlices(v.KeyRisks,normalized.KeyRisks) { return errors.New("analyst synthesis key risks must be unique and sorted") }
	return nil
}

func impactChannelsToStrings(values []ImpactChannel) []string {
	out:=make([]string,len(values));for i,v:=range values{out[i]=string(v)};return out
}

func validMaterialityLevel(v MaterialityLevel) bool {
	switch v {case MaterialityLow,MaterialityMedium,MaterialityHigh,MaterialityUnknown:return true;default:return false}
}

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"dev/bravebird/browser-automation-go/pkg/ingestion"
	"dev/bravebird/browser-automation-go/pkg/models"
	"dev/bravebird/browser-automation-go/pkg/semantic"
)

func main() {
	parser := ingestion.NewHybridParser()
	if err := parser.ParseFile("hybrid_events.json"); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Debug: check first few rrweb events
	fmt.Println("=== DEBUG: First 5 rrweb events ===")
	rrwebEvents := parser.GetRRWebEvents()
	for i, e := range rrwebEvents {
		if i >= 5 {
			break
		}
		fmt.Printf("Event %d: Source=%s, Type=%v (type: %T)\n", i, e.Source, e.Type, e.Type)

		// Check if it's a meta event
		eventType, _ := e.Type.(float64)
		if int(eventType) == models.RRWebEventMeta {
			var meta models.RRWebMetaData
			if err := json.Unmarshal(e.Data, &meta); err == nil {
				fmt.Printf("  -> META: href=%s\n", meta.Href)
			} else {
				fmt.Printf("  -> META parse error: %v\n", err)
			}
		}
	}
	fmt.Println("===================================")

	fmt.Println("Start URL from parser:", parser.GetStartURL())
	fmt.Println("Total events:", len(parser.GetEvents()))
	fmt.Println("RRWeb events:", len(parser.GetRRWebEvents()))
	fmt.Println("Custom events:", len(parser.GetCustomEvents()))

	// Debug: Print custom events
	fmt.Println("=== DEBUG: Custom Events ===")
	for i, e := range parser.GetCustomEvents() {
		fmt.Printf("Custom Event %d: Type=%v\n", i, e.Type)
		// Try to unmarshal to see content if needed, keypress/click etc
		// For now just type matches what we expect?
	}
	fmt.Println("============================")

	// Extract semantic actions
	extractor := semantic.NewExtractor(parser, semantic.ToleranceMedium)
	actions := extractor.ExtractActions()

	fmt.Println("---")
	fmt.Println("Extracted", len(actions), "semantic actions")
	fmt.Println("---")

	// Show all actions
	for i, a := range actions {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"sequence_id":      a.SequenceID,
			"action_type":      a.ActionType,
			"interaction_rank": a.InteractionRank,
			"value":            a.Value,
			"target_selector":  a.Target.Selector,
			"source":           a.Metadata["source"],
		}, "", "  ")
		fmt.Printf("Action %d: %s\n", i+1, string(data))
		// Also print small summary for others to track flow
		// fmt.Printf("Action %d: %s %s\n", i+1, a.ActionType, a.Target.Selector)
	}
}

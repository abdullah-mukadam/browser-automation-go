package ingestion

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	"dev/bravebird/browser-automation-go/pkg/models"
	pb "dev/bravebird/browser-automation-go/pkg/proto"
)

// ProtoParser parses binary protobuf files containing hybrid events
type ProtoParser struct {
	*HybridParser // Inherit functionality
}

// NewProtoParser creates a new proto parser instance
func NewProtoParser() *ProtoParser {
	return &ProtoParser{
		HybridParser: NewHybridParser(),
	}
}

// ParseFile reads and parses a binary proto file
func (p *ProtoParser) ParseFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return p.Parse(data)
}

// Parse parses binary protobuf data
func (p *ProtoParser) Parse(data []byte) error {
	var session pb.HybridSession
	if err := proto.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to unmarshal proto: %w", err)
	}

	for _, pbEvent := range session.Events {
		event, err := p.convertProtoEvent(pbEvent)
		if err != nil {
			continue
		}
		p.events = append(p.events, event)
	}

	// Sort events by timestamp
	sort.Slice(p.events, func(i, j int) bool {
		return p.events[i].Timestamp < p.events[j].Timestamp
	})

	return nil
}

// convertProtoEvent converts a protobuf event to internal model
func (p *ProtoParser) convertProtoEvent(pbEvent *pb.HybridEvent) (models.HybridEvent, error) {
	event := models.HybridEvent{
		Source:    pbEvent.Source,
		Timestamp: int64(pbEvent.Timestamp),
	}

	// Handle Type (string in proto -> interface{} in model)
	if pbEvent.Source == "rrweb" {
		// Try to convert string type to int for rrweb events
		if typeInt, err := strconv.Atoi(pbEvent.Type); err == nil {
			event.Type = typeInt
		} else {
			// Keeps as string if parsing fails, but logged or handled downstream?
			// HybridParser expects int for rrweb events to extract actions properly.
			// Let's force it or default.
			event.Type = pbEvent.Type
		}
	} else {
		event.Type = pbEvent.Type
	}

	// Handle Data (string in proto -> json.RawMessage in model)
	if pbEvent.Data != "" {
		event.Data = json.RawMessage(pbEvent.Data)
	}

	// Handle Custom Target
	if pbEvent.Target != nil {
		event.Target = &models.EventTarget{
			Tag:      pbEvent.Target.Tag,
			Selector: pbEvent.Target.Selector,
			Text:     pbEvent.Target.Text,
		}
	}

	// Handle Value
	if pbEvent.Value != "" {
		event.Value = pbEvent.Value
	}

	// Handle Text (copy/paste content) - Map to where?
	// models.HybridEvent doesn't have a top-level Text field (it has Target.Text, Value).
	// But schema.proto has `string text = 7`.
	// For copy/paste, `Value` is usually used.
	// `HybridParser.customEventToAction` uses `event.Value` for paste. Not text.
	// If `pbEvent.Text` is set, maybe assign to Value if Value is empty?
	if event.Value == "" && pbEvent.Text != "" {
		event.Value = pbEvent.Text
	}

	return event, nil
}

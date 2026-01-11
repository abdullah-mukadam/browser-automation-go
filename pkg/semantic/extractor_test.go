package semantic

import (
	"testing"

	"dev/bravebird/browser-automation-go/pkg/models"
)

func TestFilterLowValueActions(t *testing.T) {
	// Setup generic actions with different ranks
	highRankAction := models.SemanticAction{
		SequenceID:      1,
		ActionType:      models.ActionClick,
		InteractionRank: models.RankHigh,
		Target:          models.SemanticTarget{Tag: "button", Text: "Submit"},
	}

	mediumRankAction := models.SemanticAction{
		SequenceID:      2,
		ActionType:      models.ActionClick,
		InteractionRank: models.RankMedium,
		Target:          models.SemanticTarget{Tag: "div", Selector: ".btn"},
	}

	lowRankAction := models.SemanticAction{
		SequenceID:      3,
		ActionType:      models.ActionClick,
		InteractionRank: models.RankLow,
		Target:          models.SemanticTarget{Tag: "div"},
	}

	scrollAction := models.SemanticAction{
		SequenceID:      4,
		ActionType:      models.ActionScroll,
		InteractionRank: models.RankLow,
	}

	actions := []models.SemanticAction{highRankAction, mediumRankAction, lowRankAction, scrollAction}

	tests := []struct {
		name      string
		tolerance ToleranceLevel
		wantCount int
	}{
		{
			name:      "Low Tolerance - Strict (High only)",
			tolerance: ToleranceLow,
			wantCount: 1, // Only highRankAction
		},
		{
			name:      "Medium Tolerance - Default (High + Medium)",
			tolerance: ToleranceMedium,
			wantCount: 2, // highRankAction + mediumRankAction
		},
		{
			name:      "High Tolerance - Permissive (All)",
			tolerance: ToleranceHigh,
			wantCount: 4, // All actions
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Extractor{
				tolerance: tt.tolerance,
			}

			filtered := e.filterLowValueActions(actions)

			if len(filtered) != tt.wantCount {
				t.Errorf("filterLowValueActions() count = %v, want %v", len(filtered), tt.wantCount)
			}
		})
	}
}

func TestDeduplicateNavigations(t *testing.T) {
	tests := []struct {
		name    string
		actions []models.SemanticAction
		wantIDs []int
	}{
		{
			name: "Basic duplication",
			actions: []models.SemanticAction{
				{SequenceID: 1, ActionType: models.ActionNavigate, Value: "https://google.com"},
				{SequenceID: 2, ActionType: models.ActionNavigate, Value: "https://google.com"},
			},
			wantIDs: []int{1},
		},
		{
			name: "Same domain sequential",
			actions: []models.SemanticAction{
				{SequenceID: 1, ActionType: models.ActionNavigate, Value: "https://google.com"},
				{SequenceID: 2, ActionType: models.ActionNavigate, Value: "https://google.com/search"},
			},
			wantIDs: []int{1},
		},
		{
			name: "Different domain sequential",
			actions: []models.SemanticAction{
				{SequenceID: 1, ActionType: models.ActionNavigate, Value: "https://google.com"},
				{SequenceID: 2, ActionType: models.ActionNavigate, Value: "https://example.com"},
			},
			wantIDs: []int{1, 2},
		},
		{
			name: "Interaction followed by same domain navigation",
			actions: []models.SemanticAction{
				{SequenceID: 1, ActionType: models.ActionNavigate, Value: "https://google.com"},
				{SequenceID: 2, ActionType: models.ActionInput, Value: "cats"},
				{SequenceID: 3, ActionType: models.ActionNavigate, Value: "https://google.com/search?q=cats"},
			},
			wantIDs: []int{1, 2},
		},
		{
			name: "Interaction followed by different domain navigation",
			actions: []models.SemanticAction{
				{SequenceID: 1, ActionType: models.ActionNavigate, Value: "https://google.com"},
				{SequenceID: 2, ActionType: models.ActionClick, Target: models.SemanticTarget{Text: "Link"}},
				{SequenceID: 3, ActionType: models.ActionNavigate, Value: "https://example.com"},
			},
			wantIDs: []int{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Extractor{}
			got := e.deduplicateNavigations(tt.actions)

			if len(got) != len(tt.wantIDs) {
				t.Errorf("got %d actions, want %d", len(got), len(tt.wantIDs))
				return
			}

			for i, action := range got {
				if action.SequenceID != tt.wantIDs[i] {
					t.Errorf("action[%d].SequenceID = %d, want %d", i, action.SequenceID, tt.wantIDs[i])
				}
			}
		})
	}
}

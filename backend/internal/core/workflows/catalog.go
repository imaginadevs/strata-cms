package workflows

import (
	"sort"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
)

// CatalogField describes one config key for the frontend form builder.
type CatalogField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"` // string | any | object | number | boolean
	Required    bool   `json:"required"`
}

// CatalogEntry is one registered trigger or action type.
type CatalogEntry struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	ConfigFields []CatalogField `json:"config_fields"`
}

// Catalog lists all registered trigger and action types with form metadata.
func (r *Registry) Catalog() (triggers []CatalogEntry, actions []CatalogEntry) {
	triggers = make([]CatalogEntry, 0, len(r.triggers))
	for t := range r.triggers {
		triggers = append(triggers, triggerCatalogEntry(t))
	}
	sort.Slice(triggers, func(i, j int) bool { return triggers[i].Type < triggers[j].Type })

	actions = make([]CatalogEntry, 0, len(r.actions))
	for t := range r.actions {
		actions = append(actions, actionCatalogEntry(t))
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].Type < actions[j].Type })
	return triggers, actions
}

func triggerCatalogEntry(t wfdomain.TriggerType) CatalogEntry {
	switch t {
	case wfdomain.TriggerFieldChanged:
		return CatalogEntry{
			Type:        string(t),
			Name:        "Field changed",
			Description: "Fires when an item field changes to a target value (edge trigger).",
			ConfigFields: []CatalogField{
				{Key: "collection", Label: "Collection", Type: "string", Required: true},
				{Key: "field", Label: "Field", Type: "string", Required: true},
				{Key: "to", Label: "To value", Type: "any", Required: true, Description: "New value that activates the workflow"},
				{Key: "from", Label: "From value", Type: "any", Required: false, Description: "Optional: only fire if previous value matched"},
				{Key: "op", Label: "Operator", Type: "string", Required: false, Description: "Reserved; use eq"},
			},
		}
	case wfdomain.TriggerSchedule:
		return CatalogEntry{
			Type:        string(t),
			Name:        "Schedule (cron)",
			Description: "Runs on a cron schedule for items matching an optional filter.",
			ConfigFields: []CatalogField{
				{Key: "cron", Label: "Cron expression", Type: "string", Required: true, Description: "5-field cron, e.g. 0 9 * * 1-5"},
				{Key: "collection", Label: "Collection", Type: "string", Required: true},
				{Key: "timezone", Label: "Timezone", Type: "string", Required: false, Description: "IANA name; default UTC"},
				{Key: "filter", Label: "Item filter", Type: "object", Required: false},
				{Key: "limit", Label: "Max items per tick", Type: "number", Required: false},
			},
		}
	default:
		return CatalogEntry{Type: string(t), Name: string(t), Description: "Custom trigger", ConfigFields: nil}
	}
}

func actionCatalogEntry(t wfdomain.ActionType) CatalogEntry {
	switch t {
	case wfdomain.ActionEmailWithPDF:
		return CatalogEntry{
			Type:        string(t),
			Name:        "Email with PDF",
			Description: "Renders a PDF from the entity and sends it by SMTP. Supports {{item.field}} templates.",
			ConfigFields: []CatalogField{
				{Key: "to", Label: "To", Type: "string", Required: true, Description: "Recipient(s); comma-separated or {{item.email}}"},
				{Key: "subject", Label: "Subject", Type: "string", Required: true},
				{Key: "body", Label: "Body", Type: "string", Required: false},
				{Key: "cc", Label: "Cc", Type: "any", Required: false},
				{Key: "pdf", Label: "PDF options", Type: "object", Required: true, Description: "Object with template (required) and filename (optional)"},
			},
		}
	default:
		return CatalogEntry{Type: string(t), Name: string(t), Description: "Custom action", ConfigFields: nil}
	}
}

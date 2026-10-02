package ui

import (
	"fmt"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// Group order is a local display policy, not recovery of saved web display state.
func timelineGroupField(view *github.View) (github.Field, error) {
	if view == nil || (len(view.GroupByFields) == 0 && len(view.VerticalGroupBy) == 0) {
		return github.Field{}, nil
	}
	if len(view.GroupByFields) != 1 || len(view.VerticalGroupBy) != 0 {
		return github.Field{}, fmt.Errorf("timeline grouping requires one single-select field and no vertical grouping")
	}
	id := view.GroupByFields[0].ID
	field, count := github.Field{}, 0
	for _, candidate := range view.ProjectFields {
		if id != "" && candidate.ID == id {
			field, count = candidate, count+1
		}
	}
	if count != 1 || !isSingleSelectField(field) {
		return github.Field{}, fmt.Errorf("timeline grouping requires a unique complete single-select definition")
	}
	// ProjectFields is the filter-validation projection (ID/name/type only).
	// The saved grouping connection supplies the complete option definitions.
	field = view.GroupByFields[0]
	if !isSingleSelectField(field) || len(field.Options) == 0 {
		return github.Field{}, fmt.Errorf("timeline grouping requires complete single-select group options")
	}
	seen := map[string]bool{}
	for _, option := range field.Options {
		if option.ID == "" || seen[option.ID] {
			return github.Field{}, fmt.Errorf("timeline grouping option IDs must be nonempty and unique")
		}
		seen[option.ID] = true
	}
	return field, nil
}

func timelineGroups(view *github.View, items []github.Item) ([]boardLane, error) {
	field, err := timelineGroupField(view)
	if err != nil {
		return nil, err
	}
	if field.ID == "" {
		return []boardLane{{Key: "all", Items: items}}, nil
	}
	groups := make([]boardLane, 0, len(field.Options)+2)
	indexes := map[string]int{}
	for _, option := range field.Options {
		indexes[option.ID] = len(groups)
		groups = append(groups, boardLane{Key: "option:" + option.ID, Name: option.Name})
	}
	unset, unavailable := len(groups), len(groups)+1
	groups = append(groups, boardLane{Key: "unset", Name: "No value"}, boardLane{Key: "unavailable", Name: "Unavailable group"})
	for _, item := range items {
		index, count := unset, 0
		for _, value := range item.FieldValues {
			if value.FieldID != field.ID {
				continue
			}
			count++
			if !value.Available {
				index = unavailable
			} else if value.OptionID != "" {
				var found bool
				index, found = indexes[value.OptionID]
				if !found {
					index = unavailable
				}
			} else if value.Value != "" {
				index = unavailable
			}
		}
		if count > 1 {
			index = unavailable
		}
		groups[index].Items = append(groups[index].Items, item)
	}
	if len(groups[unavailable].Items) == 0 {
		groups = groups[:unavailable]
	}
	return groups, nil
}

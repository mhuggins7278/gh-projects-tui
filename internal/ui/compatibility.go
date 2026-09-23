package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type viewCompatibility struct {
	reasons []string
}

func (c viewCompatibility) supported() bool {
	return len(c.reasons) == 0
}

func (c viewCompatibility) summary() string {
	return strings.Join(c.reasons, "; ")
}

func evaluateViewCompatibility(view github.View) viewCompatibility {
	reasons := make([]string, 0, 4)
	switch view.Layout {
	case github.BoardLayout:
	case github.TableLayout:
	case github.RoadmapLayout:
		reasons = append(reasons, "roadmap views are not supported")
	default:
		reasons = append(reasons, fmt.Sprintf("layout %q is not supported", view.Layout))
	}

	if err := validateSavedFilter(view.Filter); err != nil {
		reasons = append(reasons, fmt.Sprintf("saved filter %q is unsupported: %v", view.Filter, err))
	}

	if len(view.GroupByFields) > 1 {
		reasons = append(reasons, "multiple column grouping fields are not supported")
	}
	if len(view.VerticalGroupBy) > 1 {
		reasons = append(reasons, "multiple vertical grouping fields are not supported")
	}
	for _, grouping := range []struct {
		fields []github.Field
		axis   string
	}{
		{fields: view.GroupByFields, axis: "column"},
		{fields: view.VerticalGroupBy, axis: "vertical"},
	} {
		if len(grouping.fields) == 0 {
			continue
		}
		field := grouping.fields[0]
		switch {
		case isSingleSelectField(field):
			if len(field.Options) == 0 {
				reasons = append(reasons, fmt.Sprintf("%s grouping field %q has no options", grouping.axis, field.Name))
			}
		case isIterationField(field):
			if len(field.Iterations) == 0 {
				reasons = append(reasons, fmt.Sprintf("%s grouping field %q has no iterations", grouping.axis, field.Name))
			}
		default:
			reasons = append(reasons, fmt.Sprintf("%s grouping field %q is not single-select or iteration", grouping.axis, field.Name))
		}
	}

	for _, sortField := range view.SortByFields {
		if !supportedSortDirection(sortField.Direction) {
			reasons = append(reasons, fmt.Sprintf("sort direction %q is not supported", sortField.Direction))
			continue
		}
		if !supportedSortField(sortField.Field) {
			name := sortField.Field.Name
			if name == "" {
				name = sortField.Field.DataType
			}
			reasons = append(reasons, fmt.Sprintf("sort field %q is not supported", name))
		}
	}

	return viewCompatibility{reasons: reasons}
}

func isSingleSelectField(field github.Field) bool {
	return field.Kind == "ProjectV2SingleSelectField" || strings.EqualFold(field.DataType, "SINGLE_SELECT")
}

func isIterationField(field github.Field) bool {
	return field.Kind == "ProjectV2IterationField" || strings.EqualFold(field.DataType, "ITERATION")
}

func supportedSortDirection(direction string) bool {
	direction = strings.ToUpper(strings.TrimSpace(direction))
	return direction == "ASC" || direction == "DESC"
}

func supportedSortField(field github.Field) bool {
	if field.Name == "" && field.DataType == "" && field.Kind == "" {
		return false
	}
	if field.Name == "Title" || strings.EqualFold(field.DataType, "TITLE") || strings.EqualFold(field.DataType, "POSITION") {
		return true
	}
	switch strings.ToUpper(field.DataType) {
	case "TEXT", "NUMBER", "DATE", "SINGLE_SELECT":
		return true
	case "ITERATION":
		if len(field.Iterations) == 0 {
			return false
		}
		for _, iteration := range field.Iterations {
			if _, err := time.Parse("2006-01-02", iteration.StartDate); err != nil {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func unsupportedViewStatus(view github.View, compatibility viewCompatibility) string {
	name := view.Name
	if name == "" {
		name = fmt.Sprintf("#%d", view.Number)
	}
	return fmt.Sprintf("View %q is unsupported: %s", name, compatibility.summary())
}

package application

import (
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
)

// PersonalHistoryDay identifies an ordered day of tasks in the requested time zone.
type PersonalHistoryDay struct {
	Date    string      `json:"date"`
	Title   string      `json:"title"`
	TaskIDs []uuid.UUID `json:"task_ids"`
}

// ApplyPersonalHistoryCalendar converts inclusive calendar dates into UTC query bounds.
func ApplyPersonalHistoryCalendar(input *PersonalHistoryInput, zone, from, through string) (*time.Location, error) {
	if len(zone) > 128 || zone == "Local" {
		return nil, ErrInvalidPublicQuery
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, ErrInvalidPublicQuery
	}
	if (from != "" || through != "") && (input.CreatedFrom != nil || input.CreatedBefore != nil) {
		return nil, ErrInvalidPublicQuery
	}
	if from != "" {
		start, err := time.ParseInLocation(time.DateOnly, from, location)
		if err != nil || start.Format(time.DateOnly) != from {
			return nil, ErrInvalidPublicQuery
		}
		input.CreatedFrom = &start
	}
	if through != "" {
		end, err := time.ParseInLocation(time.DateOnly, through, location)
		if err != nil || end.Format(time.DateOnly) != through {
			return nil, ErrInvalidPublicQuery
		}
		before := end.AddDate(0, 0, 1)
		input.CreatedBefore = &before
	}
	return location, input.Normalize()
}

// GroupPersonalHistoryDays preserves query order and emits one group per local day.
func GroupPersonalHistoryDays(items []PersonalHistoryItem, location *time.Location, now time.Time) []PersonalHistoryDay {
	groups := make([]PersonalHistoryDay, 0)
	indices := make(map[string]int)
	currentYear := now.In(location).Year()
	for _, item := range items {
		local := item.CreateTime.In(location)
		date := local.Format(time.DateOnly)
		index, exists := indices[date]
		if !exists {
			year, month, day := local.Date()
			title := fmt.Sprintf("%d月%d日", month, day)
			if year != currentYear {
				title = fmt.Sprintf("%d年%s", year, title)
			}
			index = len(groups)
			indices[date] = index
			groups = append(groups, PersonalHistoryDay{Date: date, Title: title})
		}
		groups[index].TaskIDs = append(groups[index].TaskIDs, item.ID)
	}
	return groups
}

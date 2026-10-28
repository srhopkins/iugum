package agentplatform

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type beadsNavigation struct {
	View          *string   `json:"view"`
	Projects      *[]string `json:"projects"`
	Search        *string   `json:"search"`
	Statuses      *[]string `json:"statuses"`
	Priorities    *[]string `json:"priorities"`
	Types         *[]string `json:"types"`
	Assignees     *[]string `json:"assignees"`
	Labels        *[]string `json:"labels"`
	Sort          *string   `json:"sort"`
	Lanes         *bool     `json:"lanes"`
	Ticket        *string   `json:"ticket"`
	TicketProject *string   `json:"ticketProject"`
}

var beadTicket = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func validateBeadsNavigation(raw json.RawMessage) error {
	var a beadsNavigation
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if e := d.Decode(&a); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(raw, &fields); e != nil || fields == nil {
		return errors.New("Beads navigation must be an object")
	}
	for _, v := range fields {
		if string(v) == "null" {
			return errors.New("Beads navigation fields cannot be null")
		}
	}
	text := func(s string, n int) bool {
		return s != "" && len(s) <= n && utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return r < 32 || r == 127 || unicode.Is(unicode.Cs, r) })
	}
	for _, list := range []*[]string{a.Projects, a.Statuses, a.Priorities, a.Types, a.Assignees, a.Labels} {
		if list != nil {
			if len(*list) > 20 {
				return errors.New("too many Beads filter values")
			}
			for _, v := range *list {
				if !text(v, 128) {
					return errors.New("invalid Beads filter value")
				}
			}
		}
	}
	for _, v := range []*string{a.Ticket, a.TicketProject} {
		if v != nil && !text(*v, 128) {
			return errors.New("invalid Beads text field")
		}
	}
	if a.Search != nil && !text(*a.Search, 512) {
		return errors.New("invalid Beads search")
	}
	if a.Ticket != nil && !beadTicket.MatchString(*a.Ticket) {
		return errors.New("invalid Beads ticket")
	}
	if a.TicketProject != nil && a.Ticket == nil {
		return errors.New("ticket project requires a ticket")
	}
	if a.View != nil && !oneOf(*a.View, "board", "tree", "activity", "graph", "list") {
		return errors.New("invalid Beads view")
	}
	if a.Sort != nil && !oneOf(*a.Sort, "updated", "created", "priority", "status") {
		return errors.New("invalid Beads sort")
	}
	return nil
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

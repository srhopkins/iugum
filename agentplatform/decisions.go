package agentplatform

import (
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"strings"
)

// Native response validation is shared by the HTTP boundary and adapters.
func validateNativeAnswer(q contract.AgentSessionRequest, result json.RawMessage) error {
	var v map[string]json.RawMessage
	if json.Unmarshal(result, &v) != nil || v == nil {
		return errors.New("invalid decision object")
	}
	switch q.Method {
	case "can_use_tool":
		var b string
		_ = json.Unmarshal(v["behavior"], &b)
		if b == "allow" && len(v) == 2 {
			var input map[string]json.RawMessage
			if json.Unmarshal(v["updatedInput"], &input) == nil && input != nil {
				return nil
			}
		}
		if b == "deny" && len(v) == 2 {
			var msg string
			if json.Unmarshal(v["message"], &msg) == nil {
				return nil
			}
		}
		return errors.New("explicit allow with updatedInput or deny with message required")
	case "permission.asked":
		var reply string
		if len(v) == 1 && json.Unmarshal(v["reply"], &reply) == nil && (reply == "once" || reply == "reject") {
			return nil
		}
		return errors.New("one-time permission or reject required")
	case "question.asked":
		var reject bool
		if len(v) == 1 && json.Unmarshal(v["reject"], &reject) == nil && reject {
			return nil
		}
		var answers [][]string
		if len(v) != 1 || json.Unmarshal(v["answers"], &answers) != nil || answers == nil {
			return errors.New("answers or explicit rejection required")
		}
		var p struct {
			Questions []struct {
				Options  []struct{ Label string }
				Multiple bool
				Custom   *bool
			}
		}
		if json.Unmarshal(q.Params, &p) != nil || len(p.Questions) == 0 || len(answers) != len(p.Questions) {
			return errors.New("answer every requested question")
		}
		for i, question := range p.Questions {
			if len(answers[i]) == 0 || !question.Multiple && len(answers[i]) != 1 {
				return errors.New("question answer count mismatch")
			}
			seen := map[string]bool{}
			for _, answer := range answers[i] {
				if strings.TrimSpace(answer) == "" || seen[answer] {
					return errors.New("empty or duplicate answer")
				}
				seen[answer] = true
				listed := false
				for _, o := range question.Options {
					if o.Label == answer {
						listed = true
					}
				}
				if !listed && question.Custom != nil && !*question.Custom {
					return errors.New("answer must use a listed option")
				}
			}
		}
		return nil
	}
	return errors.New("unsupported native decision")
}

func validateCursorQuestion(q contract.AgentSessionRequest, result json.RawMessage) error {
	var response struct {
		Outcome struct {
			Outcome string `json:"outcome"`
			Answers []struct {
				QuestionID string   `json:"questionId"`
				Selected   []string `json:"selectedOptionIds"`
			} `json:"answers"`
		} `json:"outcome"`
	}
	if json.Unmarshal(result, &response) != nil {
		return errors.New("invalid Cursor question response")
	}
	if response.Outcome.Outcome == "cancelled" || response.Outcome.Outcome == "skipped" {
		return nil
	}
	if response.Outcome.Outcome != "answered" {
		return errors.New("invalid Cursor question outcome")
	}
	var params struct {
		Questions []struct {
			ID      string `json:"id"`
			Options []struct {
				ID string `json:"id"`
			} `json:"options"`
			Multiple bool `json:"allowMultiple"`
		} `json:"questions"`
	}
	if json.Unmarshal(q.Params, &params) != nil || len(params.Questions) == 0 || len(response.Outcome.Answers) != len(params.Questions) {
		return errors.New("answer every Cursor question")
	}
	used := map[string]bool{}
	for _, a := range response.Outcome.Answers {
		if a.QuestionID == "" || used[a.QuestionID] {
			return errors.New("unknown or duplicate Cursor question")
		}
		used[a.QuestionID] = true
		found := false
		for _, question := range params.Questions {
			if question.ID != a.QuestionID {
				continue
			}
			found = true
			if len(a.Selected) == 0 || !question.Multiple && len(a.Selected) != 1 {
				return errors.New("Cursor question answer count mismatch")
			}
			seen := map[string]bool{}
			for _, choice := range a.Selected {
				valid := false
				for _, o := range question.Options {
					if o.ID == choice {
						valid = true
					}
				}
				if !valid || seen[choice] {
					return errors.New("Cursor answer must select listed option IDs")
				}
				seen[choice] = true
			}
		}
		if !found {
			return errors.New("unknown Cursor question")
		}
	}
	return nil
}
func validateCodexQuestions(q contract.AgentSessionRequest, result json.RawMessage) error {
	var params struct {
		Questions []struct {
			ID      string `json:"id"`
			Other   bool   `json:"isOther"`
			Options []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	var response struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if json.Unmarshal(q.Params, &params) != nil || json.Unmarshal(result, &response) != nil || len(params.Questions) == 0 || len(response.Answers) != len(params.Questions) {
		return errors.New("answer every requested Codex question")
	}
	used := map[string]bool{}
	for _, question := range params.Questions {
		a, ok := response.Answers[question.ID]
		if !ok || question.ID == "" || used[question.ID] || len(a.Answers) != 1 {
			return errors.New("unknown, missing or duplicate question identity")
		}
		used[question.ID] = true
		answer := a.Answers[0]
		if strings.TrimSpace(answer) == "" {
			return errors.New("empty question answer")
		}
		listed := false
		for _, o := range question.Options {
			if o.Label == answer {
				listed = true
			}
		}
		if len(question.Options) > 0 && !listed && !question.Other {
			return errors.New("answer must use a listed option")
		}
	}
	return nil
}

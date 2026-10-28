package agentplatform

// Conditional controls are explicit proposals, never transcript interpreters.
// Preparation returns a server-bound immutable payload; commit echoes it.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
)

type controlOrigin struct {
	Conversation string `json:"conversation"`
	CommandID    string `json:"commandId"`
}
type canonicalControl struct {
	Phase         string                    `json:"phase"`
	Incarnation   string                    `json:"incarnation"`
	Window        string                    `json:"window"`
	CaptureEpoch  uint64                    `json:"captureEpoch"`
	ActionID      string                    `json:"actionId"`
	Revision      uint64                    `json:"revision"`
	Session       contract.AgentSessionRef  `json:"session"`
	TargetSession *contract.AgentSessionRef `json:"targetSession,omitempty"`
	Kind          string                    `json:"kind"`
	Model         string                    `json:"model,omitempty"`
	ModelsDigest  string                    `json:"modelsDigest,omitempty"`
	RequestID     string                    `json:"requestId,omitempty"`
	RequestDigest string                    `json:"requestDigest,omitempty"`
	Result        json.RawMessage           `json:"result,omitempty"`
	ExpiresAt     time.Time                 `json:"expiresAt"`
	Digest        string                    `json:"digest"`
}
type controlReceipt struct {
	Proposal                            canonicalControl
	Conversation, Digest, Status, Error string
}
type canonicalControls struct {
	mu          sync.Mutex
	incarnation string
	receipts    map[string]*controlReceipt
}

var controlWindow = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func controlDigest(id string, q canonicalControl) string {
	q.Phase = ""
	q.Digest = ""
	b, _ := json.Marshal(struct {
		Conversation string
		Proposal     canonicalControl
	}{id, q})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func modelsDigest(m contract.AgentSessionModels) string {
	b, _ := json.Marshal(struct {
		Options   []contract.AgentSessionModel
		Current   string
		CanSelect bool
	}{m.Options, m.Current, m.CanSelect})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func controlResult(r *controlReceipt) map[string]string {
	return map[string]string{"actionId": r.Proposal.ActionID, "status": r.Status, "error": r.Error}
}
func (s *Server) controlSession(id string, q canonicalControl) (*managed, error) {
	s.convMu.Lock()
	defer s.convMu.Unlock()
	c, ok := s.conversations[id]
	if !ok || c.Revision != q.Revision || !sameSession(c.Selected, q.Session) {
		return nil, errors.New("conversation target changed; prepare again")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.sessions[q.Session.Provider+":"+q.Session.ID]
	if s.closed || m == nil {
		return nil, errors.New("session unavailable")
	}
	return m, nil
}
func validControl(q canonicalControl) error {
	if !controlWindow.MatchString(q.Window) || !uuid.MatchString(q.ActionID) || q.Revision == 0 {
		return errors.New("explicit window, action and revision required")
	}
	if err := validRef(q.Session); err != nil {
		return err
	}
	if q.Kind != "switch" && q.TargetSession != nil {
		return errors.New("target session is only valid for switch control")
	}
	switch q.Kind {
	case "switch":
		if q.TargetSession == nil || q.Model != "" || q.ModelsDigest != "" || q.RequestID != "" || q.RequestDigest != "" || len(q.Result) > 0 {
			return errors.New("exact existing linked switch required")
		}
		if err := validRef(*q.TargetSession); err != nil {
			return err
		}
	case "model":
		if q.Model == "" || q.RequestID != "" || q.RequestDigest != "" || len(q.Result) > 0 {
			return errors.New("exact model control required")
		}
	case "request":
		if q.Model != "" || q.ModelsDigest != "" || q.RequestID == "" || q.RequestDigest == "" || len(q.Result) == 0 {
			return errors.New("exact request control required")
		}
	default:
		return errors.New("unknown canonical control")
	}
	return nil
}
func (s *Server) canonicalControl(r *http.Request, id string) (any, error) {
	var q canonicalControl
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("invalid or oversized control body")
	}
	if err := strictControlJSON(raw, &q); err != nil {
		return nil, err
	}
	if err := validControl(q); err != nil {
		return nil, err
	}
	if q.Phase != "prepare" && q.Phase != "commit" {
		return nil, errors.New("explicit prepare or commit required")
	}
	s.controls.mu.Lock()
	incarnation := s.controls.incarnation
	s.controls.mu.Unlock()
	if q.Incarnation != incarnation {
		return nil, errors.New("service incarnation changed; delivery may be uncertain; do not replay")
	}
	if s.cfg.Check != nil {
		action := canonicalControlAction(q.Kind)
		if err := s.cfg.Check(r.Context(), "agent/sessions/"+q.Session.Provider+"/"+q.Session.ID, action); err != nil {
			return nil, errors.New("control policy denied")
		}
		if q.Kind == "switch" {
			if err := s.cfg.Check(r.Context(), "agent/sessions/"+q.TargetSession.Provider+"/"+q.TargetSession.ID, "select"); err != nil {
				return nil, errors.New("target selection policy denied")
			}
		}
	}
	if q.Phase == "prepare" {
		return s.prepareControl(r.Context(), id, q)
	}
	return s.commitControl(r.Context(), id, q)
}
func (s *Server) prepareControl(ctx context.Context, id string, q canonicalControl) (any, error) {
	if q.Digest != "" || !q.ExpiresAt.IsZero() || q.ModelsDigest != "" {
		return nil, errors.New("prepare cannot supply server binding")
	}
	m, err := s.controlSession(id, q)
	if err != nil {
		return nil, err
	}
	if q.Kind == "switch" {
		target, err := s.prepareLinkedSwitch(id, q)
		if err != nil {
			return nil, err
		}
		q.TargetSession = &target
	} else if q.Kind == "model" {
		m.mu.Lock()
		busy := m.busy
		m.mu.Unlock()
		if busy {
			return nil, errors.New("cannot prepare a model change during a turn")
		}
		models, err := sessionModels(ctx, m)
		if err != nil {
			return nil, errors.New("model catalogue unavailable")
		}
		if !hasControlModel(models, q.Model) {
			return nil, errors.New("model unavailable")
		}
		q.ModelsDigest = modelsDigest(models)
	} else {
		s.convMu.Lock()
		originConversation := s.conversations[id]
		m.mu.Lock()
		a := m.requests[q.RequestID]
		err = validateControlRequest(a, originConversation, q)
		m.mu.Unlock()
		s.convMu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	// Revalidate target after provider catalogue I/O, then freeze for at most30s.
	if _, err = s.controlSession(id, q); err != nil {
		return nil, err
	}
	q.ExpiresAt = time.Now().UTC().Add(30 * time.Second)
	q.Digest = controlDigest(id, q)
	s.controls.mu.Lock()
	defer s.controls.mu.Unlock()
	if q.Incarnation != s.controls.incarnation {
		return nil, errors.New("service incarnation changed")
	}
	if _, exists := s.controls.receipts[q.ActionID]; exists {
		return nil, errors.New("action identity already reserved; do not replay")
	}
	if len(s.controls.receipts) >= 4096 {
		return nil, errors.New("control receipt capacity reached; inspect state before restarting")
	}
	s.controls.receipts[q.ActionID] = &controlReceipt{Proposal: q, Conversation: id, Digest: q.Digest, Status: "prepared"}
	return q, nil
}
func hasControlModel(models contract.AgentSessionModels, id string) bool {
	if !models.CanSelect {
		return false
	}
	for _, v := range models.Options {
		if v.ID == id && id != "" {
			return true
		}
	}
	return false
}
func validateControlRequest(a *approval, conversation contract.AgentConversation, q canonicalControl) error {
	id := conversation.ID
	if a == nil || a.context.Err() != nil || a.Digest != q.RequestDigest {
		return errors.New("approval expired or changed")
	}
	if a.Origin == nil || a.Origin.Conversation != id || a.Origin.CommandID == "" {
		return errors.New("approval canonical origin unknown or different")
	}
	active := false
	for _, command := range conversation.Commands {
		if command.ID == a.Origin.CommandID && command.Status == "running" && sameSession(command.Session, q.Session) {
			active = true
			break
		}
	}
	if !active {
		return errors.New("approval originating command is no longer active")
	}
	// Existing validator rejects unsupported methods, unoffered choices and
	// persistent permission grants. No arbitrary reply object is inferred.
	return validateConditionalAnswer(a.AgentSessionRequest, q.Result)
}
func (s *Server) commitControl(ctx context.Context, id string, q canonicalControl) (any, error) {
	s.controls.mu.Lock()
	receipt := s.controls.receipts[q.ActionID]
	if q.Incarnation != s.controls.incarnation || receipt == nil || receipt.Conversation != id || q.Digest != receipt.Digest || controlDigest(id, q) != receipt.Digest {
		s.controls.mu.Unlock()
		return nil, errors.New("control binding changed or unknown")
	}
	if receipt.Status != "prepared" {
		result := controlResult(receipt)
		s.controls.mu.Unlock()
		return result, nil
	}
	fail := func(err error) (any, error) {
		receipt.Status = "rejected"
		receipt.Error = err.Error()
		s.controls.mu.Unlock()
		return nil, err
	}
	if !time.Now().Before(q.ExpiresAt) {
		return fail(errors.New("control proposal expired"))
	}
	// Linearize canonical target validation and exact native reservation together.
	// Lock order matches existing prompt/switch paths: conversation, service, session.
	s.convMu.Lock()
	c, ok := s.conversations[id]
	if !ok || c.Revision != q.Revision || !sameSession(c.Selected, q.Session) {
		s.convMu.Unlock()
		return fail(errors.New("conversation target changed; prepare again"))
	}
	s.mu.Lock()
	m := s.sessions[q.Session.Provider+":"+q.Session.ID]
	if s.closed || m == nil {
		s.mu.Unlock()
		s.convMu.Unlock()
		return fail(errors.New("session unavailable"))
	}
	if q.Kind == "switch" {
		return s.commitLinkedSwitchLocked(ctx, id, q, c, m, receipt, fail)
	}
	m.mu.Lock()
	if q.Kind == "request" {
		a := m.requests[q.RequestID]
		if err := validateControlRequest(a, c, q); err != nil {
			m.mu.Unlock()
			s.mu.Unlock()
			s.convMu.Unlock()
			return fail(err)
		}
		if err := s.authorizeControlReservation(ctx, id, q); err != nil {
			m.mu.Unlock()
			s.mu.Unlock()
			s.convMu.Unlock()
			return fail(err)
		}
		delete(m.requests, q.RequestID)
		receipt.Status = "completed"
		a.answer <- append(json.RawMessage(nil), q.Result...)
		m.mu.Unlock()
		s.mu.Unlock()
		s.convMu.Unlock()
		result := controlResult(receipt)
		s.controls.mu.Unlock()
		return result, nil
	}
	if m.busy {
		m.mu.Unlock()
		s.mu.Unlock()
		s.convMu.Unlock()
		return fail(errors.New("cannot change model during a turn"))
	}
	controller, ok := m.client.(contract.AgentSessionModelController)
	if !ok {
		m.mu.Unlock()
		s.mu.Unlock()
		s.convMu.Unlock()
		return fail(errors.New("model control unavailable"))
	}
	if err := s.authorizeControlReservation(ctx, id, q); err != nil {
		m.mu.Unlock()
		s.mu.Unlock()
		s.convMu.Unlock()
		return fail(err)
	}
	m.busy = true
	receipt.Status = "running"
	s.wg.Add(1)
	m.mu.Unlock()
	s.mu.Unlock()
	s.convMu.Unlock()
	s.controls.mu.Unlock()
	defer s.wg.Done()
	// No service/conversation/session/receipt lock is held during provider I/O.
	models, err := controller.Models(ctx)
	attempted := false
	if err == nil && (modelsDigest(models) != q.ModelsDigest || !hasControlModel(models, q.Model)) {
		err = errors.New("model catalogue or current selection changed")
	}
	if err == nil {
		attempted = true
		err = controller.SelectModel(ctx, q.Model)
	}
	if err == nil {
		b, _ := json.Marshal(struct {
			Ref   contract.AgentSessionRef `json:"ref"`
			Model string                   `json:"model"`
		}{q.Session, q.Model})
		err = atomicFile(filepath.Join(s.cfg.DataDir, "model-"+q.Session.Provider+"-"+q.Session.ID+".json"), b)
	}
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
	s.controls.mu.Lock()
	defer s.controls.mu.Unlock()
	if err == nil {
		receipt.Status = "completed"
	} else {
		receipt.Status = "rejected"
		if attempted {
			receipt.Status = "uncertain"
		}
		receipt.Error = "control was not confirmed; inspect native state before preparing again"
	}
	return controlResult(receipt), nil
}

// Expanded spoken controls deliberately support less than the manual dialogs:
// once-only permissions and a single listed, nonsecret question option.
func validateConditionalAnswer(q contract.AgentSessionRequest, result json.RawMessage) error {
	if err := validateAnswer(q, result); err != nil {
		return err
	}
	switch q.Method {
	case "can_use_tool":
		var answer struct {
			Behavior string          `json:"behavior"`
			Input    json.RawMessage `json:"updatedInput"`
		}
		var params struct {
			Input json.RawMessage `json:"input"`
		}
		_ = json.Unmarshal(result, &answer)
		_ = json.Unmarshal(q.Params, &params)
		if answer.Behavior == "allow" {
			var a, b any
			if json.Unmarshal(answer.Input, &a) != nil || json.Unmarshal(params.Input, &b) != nil {
				return errors.New("original tool input required")
			}
			x, _ := json.Marshal(a)
			y, _ := json.Marshal(b)
			if string(x) != string(y) {
				return errors.New("spoken control cannot modify tool input")
			}
		}
		return nil
	case "permission.asked":
		return nil
	case "session/request_permission":
		var value struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId,omitempty"`
			} `json:"outcome"`
		}
		if strictControlJSON(result, &value) != nil {
			return errors.New("invalid one-time permission shape")
		}
		return nil
	case "question.asked":
		var params struct {
			Questions []struct {
				Options  []struct{ Label string }
				Multiple bool
				Secret   bool `json:"isSecret"`
			}
		}
		var answer struct{ Answers [][]string }
		if json.Unmarshal(q.Params, &params) != nil || json.Unmarshal(result, &answer) != nil || len(params.Questions) != 1 || len(answer.Answers) != 1 || len(answer.Answers[0]) != 1 || params.Questions[0].Multiple || params.Questions[0].Secret {
			return errors.New("use manual dialog for this question")
		}
		for _, o := range params.Questions[0].Options {
			if o.Label == answer.Answers[0][0] {
				return nil
			}
		}
		return errors.New("listed question option required")
	case "cursor/ask_question":
		var value struct {
			Outcome struct {
				Outcome string `json:"outcome"`
				Answers []struct {
					QuestionID string   `json:"questionId"`
					Selected   []string `json:"selectedOptionIds"`
				} `json:"answers,omitempty"`
			} `json:"outcome"`
		}
		if strictControlJSON(result, &value) != nil {
			return errors.New("invalid question answer shape")
		}
		var params struct {
			Questions []struct {
				Options  []struct{ ID string }
				Multiple bool `json:"allowMultiple"`
				Secret   bool `json:"isSecret"`
			}
		}
		if json.Unmarshal(q.Params, &params) != nil || len(params.Questions) != 1 || len(params.Questions[0].Options) == 0 || params.Questions[0].Multiple || params.Questions[0].Secret {
			return errors.New("use manual dialog for this question")
		}
		return nil
	case "item/tool/requestUserInput":
		var value struct {
			Answers map[string]struct {
				Answers []string `json:"answers"`
			} `json:"answers"`
		}
		if strictControlJSON(result, &value) != nil {
			return errors.New("invalid question answer shape")
		}
		var params struct {
			Questions []struct {
				Options  []struct{ Label string }
				Secret   bool `json:"isSecret"`
				Multiple bool `json:"multiple"`
			}
		}
		if json.Unmarshal(q.Params, &params) != nil || len(params.Questions) != 1 || len(params.Questions[0].Options) == 0 || params.Questions[0].Secret || params.Questions[0].Multiple {
			return errors.New("use manual dialog for this question")
		}
		for _, answer := range value.Answers {
			if len(answer.Answers) != 1 {
				return errors.New("single listed answer required")
			}
			for _, option := range params.Questions[0].Options {
				if option.Label == answer.Answers[0] {
					return nil
				}
			}
		}
		return errors.New("listed question option required")
	}
	switch q.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "iugum/requestApproval":
		var fields map[string]json.RawMessage
		if json.Unmarshal(result, &fields) == nil && len(fields) == 1 && fields["decision"] != nil {
			return nil
		}
	}
	return errors.New("use manual dialog for this request")
}

func strictControlJSON(raw json.RawMessage, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("extra JSON data")
	}
	return nil
}

// Called while canonical/service/native locks hold the exact target reservation.
// A handler-level check alone goes stale while waiting for those locks.
func (s *Server) authorizeControlReservation(ctx context.Context, id string, q canonicalControl) error {
	if err := ctx.Err(); err != nil {
		return errors.New("control request cancelled before reservation")
	}
	if !time.Now().Before(q.ExpiresAt) {
		return errors.New("control proposal expired before reservation")
	}
	if err := s.conversationCheck(ctx, id, "write"); err != nil {
		return errors.New("conversation control policy denied before reservation")
	}
	if s.cfg.Check != nil {
		if err := s.cfg.Check(ctx, "agent/sessions/api/conversations/"+id+"/control", "write"); err != nil {
			return errors.New("control route policy denied before reservation")
		}
		action := canonicalControlAction(q.Kind)
		if err := s.cfg.Check(ctx, "agent/sessions/"+q.Session.Provider+"/"+q.Session.ID, action); err != nil {
			return errors.New("native control policy denied before reservation")
		}
		if q.Kind == "switch" {
			if err := s.cfg.Check(ctx, "agent/sessions/"+q.TargetSession.Provider+"/"+q.TargetSession.ID, "select"); err != nil {
				return errors.New("target select policy denied before reservation")
			}
		}
	}
	// Policy evaluation may itself take time. Expiry/cancellation are checked at
	// the final reservation boundary too, before busy or request ownership changes.
	if ctx.Err() != nil {
		return errors.New("control request cancelled before reservation")
	}
	if !time.Now().Before(q.ExpiresAt) {
		return errors.New("control proposal expired before reservation")
	}
	return nil
}

func canonicalControlAction(kind string) string {
	switch kind {
	case "model":
		return "select-model"
	case "switch":
		return "select"
	default:
		return "approve"
	}
}

func existingControlLink(c contract.AgentConversation, target contract.AgentSessionRef) (contract.AgentSessionRef, error) {
	for _, link := range c.Links {
		if sameSession(link, target) {
			return link, nil
		}
	}
	return contract.AgentSessionRef{}, errors.New("target must be an existing canonical link")
}

// Preparation resolves only already-owned links. It never opens a native owner.
func (s *Server) prepareLinkedSwitch(id string, q canonicalControl) (contract.AgentSessionRef, error) {
	s.convMu.Lock()
	defer s.convMu.Unlock()
	c, ok := s.conversations[id]
	if !ok || c.Revision != q.Revision || !sameSession(c.Selected, q.Session) {
		return contract.AgentSessionRef{}, errors.New("source conversation changed")
	}
	target, err := existingControlLink(c, *q.TargetSession)
	if err != nil {
		return target, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sourceOwner, targetOwner := s.sessions[q.Session.Provider+":"+q.Session.ID], s.sessions[target.Provider+":"+target.ID]
	if s.closed || sourceOwner == nil || targetOwner == nil {
		return target, errors.New("existing source and target owners required; open manually first")
	}
	sourceOwner.mu.Lock()
	defer sourceOwner.mu.Unlock()
	if targetOwner != sourceOwner {
		targetOwner.mu.Lock()
		defer targetOwner.mu.Unlock()
	}
	if !sourceOwner.ready || !targetOwner.ready || !sameSession(sourceOwner.ref, q.Session) || !sameSession(targetOwner.ref, target) {
		return target, errors.New("existing native owner identity unavailable")
	}
	return target, nil
}

// Entry holds receipt, canonical and service locks. No provider operation occurs.
func (s *Server) commitLinkedSwitchLocked(ctx context.Context, id string, q canonicalControl, c contract.AgentConversation, source *managed, receipt *controlReceipt, fail func(error) (any, error)) (any, error) {
	reject := func(err error) (any, error) { s.mu.Unlock(); s.convMu.Unlock(); return fail(err) }
	target, err := existingControlLink(c, *q.TargetSession)
	if err != nil {
		return reject(err)
	}
	targetOwner := s.sessions[target.Provider+":"+target.ID]
	if targetOwner == nil {
		return reject(errors.New("target owner unavailable; no automatic resume"))
	}
	source.mu.Lock()
	if targetOwner != source {
		targetOwner.mu.Lock()
	}
	unlockOwners := func() {
		if targetOwner != source {
			targetOwner.mu.Unlock()
		}
		source.mu.Unlock()
	}
	if !source.ready || !targetOwner.ready || !sameSession(source.ref, q.Session) || !sameSession(targetOwner.ref, target) {
		unlockOwners()
		return reject(errors.New("native owner identity changed"))
	}
	if err := s.authorizeControlReservation(ctx, id, q); err != nil {
		unlockOwners()
		return reject(err)
	}
	// This is the one-time acceptance point. Busy sessions keep running unchanged.
	receipt.Status = "running"
	updated := copyConversation(c)
	updated.Selected = target
	updated.Revision++
	if err := s.putConversation(updated); err != nil {
		receipt.Status = "uncertain"
		receipt.Error = "selection persistence uncertain; inspect canonical state; do not replay"
	} else {
		receipt.Status = "completed"
		s.selected[target.Provider] = target.ID
	}
	result := controlResult(receipt)
	unlockOwners()
	s.mu.Unlock()
	s.convMu.Unlock()
	s.controls.mu.Unlock()
	return result, nil
}
